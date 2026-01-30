package services

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"tct_scrooper/models"
	"tct_scrooper/storage"
)

// MatchService handles property deduplication and matching
type MatchService struct {
	store *storage.PostgresStore
}

// NewMatchService creates a new MatchService
func NewMatchService(store *storage.PostgresStore) *MatchService {
	return &MatchService{store: store}
}

// propertyMatchCandidate is used for matching queries
type propertyMatchCandidate struct {
	ID           uuid.UUID
	Fingerprint  string
	AddressFull  string
	City         string
	PostalCode   string
	Beds         *int
	Baths        *int
	SqFt         *int
	PropertyType string
}

// InsertPotentialMatches finds and inserts potential duplicate properties
func (s *MatchService) InsertPotentialMatches(ctx context.Context, incoming *models.DomainProperty) (int, error) {
	if incoming == nil || incoming.AddressFull == "" {
		return 0, nil
	}

	normalized := strings.TrimSpace(strings.ToLower(incoming.AddressFull))
	prefix := addressPrefix(normalized, 2)
	if incoming.PostalCode == "" && prefix == "" {
		return 0, nil
	}

	// Build query to find potential matches
	query := `
		SELECT id, fingerprint, address_full, city, postal_code, beds, baths, sqft, property_type
		FROM properties
		WHERE id != $1`
	args := []interface{}{incoming.ID}
	argNum := 2

	if incoming.City != "" {
		query += " AND city = $" + itoa(argNum)
		args = append(args, incoming.City)
		argNum++
	}
	if incoming.PostalCode != "" {
		query += " AND postal_code = $" + itoa(argNum)
		args = append(args, incoming.PostalCode)
		argNum++
	}
	if prefix != "" {
		query += " AND LOWER(address_full) LIKE $" + itoa(argNum)
		args = append(args, prefix+"%")
		argNum++
	}

	rows, err := s.store.Pool().Query(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	baseIncoming := baseAddress(normalized)
	inserted := 0
	now := time.Now()

	for rows.Next() {
		var candidate propertyMatchCandidate
		if err := rows.Scan(
			&candidate.ID, &candidate.Fingerprint, &candidate.AddressFull,
			&candidate.City, &candidate.PostalCode, &candidate.Beds,
			&candidate.Baths, &candidate.SqFt, &candidate.PropertyType,
		); err != nil {
			return inserted, err
		}

		confidence, reasons, ok := scorePotentialMatch(incoming, &candidate, baseIncoming)
		if !ok {
			continue
		}

		reasonsJSON, _ := json.Marshal(reasons)
		match := &models.PropertyMatch{
			MatchedID:    candidate.ID,
			IncomingID:   incoming.ID,
			Confidence:   float32(confidence),
			MatchReasons: reasonsJSON,
			Status:       "pending",
			CreatedAt:    now,
		}

		if err := s.store.InsertPropertyMatch(ctx, match); err != nil {
			return inserted, err
		}
		inserted++
	}

	return inserted, rows.Err()
}

// scorePotentialMatch calculates a confidence score for a potential match
func scorePotentialMatch(incoming *models.DomainProperty, candidate *propertyMatchCandidate, baseIncoming string) (float64, []string, bool) {
	reasons := []string{}
	strongAddress := false
	sameAddress := false

	incomingNorm := strings.TrimSpace(strings.ToLower(incoming.AddressFull))
	candidateNorm := strings.TrimSpace(strings.ToLower(candidate.AddressFull))

	if incomingNorm != "" && candidateNorm != "" && incomingNorm == candidateNorm {
		reasons = append(reasons, "same_address")
		strongAddress = true
		sameAddress = true
	} else if baseIncoming != "" {
		baseCandidate := baseAddress(candidateNorm)
		if baseCandidate != "" && baseCandidate == baseIncoming {
			reasons = append(reasons, "same_base_address")
			strongAddress = true
		}
	}

	samePostal := incoming.PostalCode != "" && candidate.PostalCode != "" &&
		incoming.PostalCode == candidate.PostalCode
	if samePostal {
		reasons = append(reasons, "same_postal")
	}

	sameType := incoming.PropertyType != "" && candidate.PropertyType != "" &&
		strings.EqualFold(incoming.PropertyType, candidate.PropertyType)
	if sameType {
		reasons = append(reasons, "same_property_type")
	}

	closeAttrCount := 0
	if incoming.Beds != nil && candidate.Beds != nil {
		diff := absInt(*incoming.Beds - *candidate.Beds)
		if diff == 0 {
			reasons = append(reasons, "same_beds")
			closeAttrCount++
		} else if diff == 1 {
			reasons = append(reasons, "close_beds")
			closeAttrCount++
		}
	}
	if incoming.Baths != nil && candidate.Baths != nil {
		diff := absInt(*incoming.Baths - *candidate.Baths)
		if diff == 0 {
			reasons = append(reasons, "same_baths")
			closeAttrCount++
		} else if diff == 1 {
			reasons = append(reasons, "close_baths")
			closeAttrCount++
		}
	}
	if incoming.SqFt != nil && candidate.SqFt != nil && closeSqFt(*incoming.SqFt, *candidate.SqFt) {
		reasons = append(reasons, "close_sqft")
		closeAttrCount++
	}

	if !strongAddress {
		if !(samePostal && sameType && closeAttrCount >= 2) {
			return 0, nil, false
		}
		confidence := 0.55 + 0.05*float64(closeAttrCount)
		if confidence > 0.85 {
			confidence = 0.85
		}
		return confidence, reasons, true
	}

	confidence := 0.75
	if sameAddress {
		confidence = 0.9
	}
	confidence += 0.03 * float64(closeAttrCount)
	if samePostal {
		confidence += 0.03
	}
	if sameType {
		confidence += 0.03
	}
	if confidence > 0.95 {
		confidence = 0.95
	}

	return confidence, reasons, true
}

// addressPrefix returns the first N tokens of a normalized address
func addressPrefix(normalized string, minTokens int) string {
	parts := strings.Fields(normalized)
	if len(parts) < minTokens {
		return ""
	}
	return strings.Join(parts[:minTokens], " ")
}

// baseAddress strips unit numbers and trailing numbers from an address
func baseAddress(normalized string) string {
	parts := strings.Fields(normalized)
	if len(parts) == 0 {
		return ""
	}

	unitTokens := map[string]bool{
		"apt":  true,
		"unit": true,
		"ste":  true,
		"fl":   true,
		"bldg": true,
	}

	for i, part := range parts {
		if unitTokens[part] {
			parts = parts[:i]
			break
		}
	}

	if len(parts) >= 4 && isNumericToken(parts[len(parts)-1]) {
		parts = parts[:len(parts)-1]
	}

	return strings.Join(parts, " ")
}

func isNumericToken(token string) bool {
	if token == "" {
		return false
	}
	for _, r := range token {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func closeSqFt(a, b int) bool {
	if a <= 0 || b <= 0 {
		return false
	}
	diff := absInt(a - b)
	if diff <= 200 {
		return true
	}
	maxVal := a
	if b > maxVal {
		maxVal = b
	}
	return float64(diff) <= 0.1*float64(maxVal)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
