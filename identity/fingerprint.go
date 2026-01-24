package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"tct_scrooper/models"
)

var (
	streetReplacements = map[string]string{
		"street":    "st",
		"avenue":    "ave",
		"drive":     "dr",
		"road":      "rd",
		"boulevard": "blvd",
		"lane":      "ln",
		"court":     "ct",
		"place":     "pl",
		"circle":    "cir",
		"crescent":  "cres",
		"terrace":   "ter",
		"highway":   "hwy",
		"parkway":   "pkwy",
		"square":    "sq",
		"north":     "n",
		"south":     "s",
		"east":      "e",
		"west":      "w",
		"northeast": "ne",
		"northwest": "nw",
		"southeast": "se",
		"southwest": "sw",
		"apartment": "apt",
		"suite":     "ste",
		"unit":      "unit",
		"floor":     "fl",
		"building":  "bldg",
	}
	multiSpaceRegex = regexp.MustCompile(`\s+`)
	nonAlnumRegex   = regexp.MustCompile(`[^a-z0-9\s]`)
)

func Fingerprint(listing *models.RawListing) string {
	normalized := NormalizeAddress(listing.Address)
	input := fmt.Sprintf("%s|%d|%d|%d|%s",
		normalized,
		listing.Beds,
		listing.Baths,
		listing.SqFt,
		strings.ToLower(listing.PropertyType),
	)
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:16])
}

func NormalizeAddress(addr string) string {
	addr = strings.ToLower(strings.TrimSpace(addr))
	addr = nonAlnumRegex.ReplaceAllString(addr, " ")
	for full, abbrev := range streetReplacements {
		addr = strings.ReplaceAll(addr, full, abbrev)
	}
	addr = multiSpaceRegex.ReplaceAllString(addr, " ")
	return strings.TrimSpace(addr)
}
