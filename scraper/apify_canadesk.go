package scraper

import (
	"encoding/json"
	"fmt"
	"strings"

	"tct_scrooper/config"
	"tct_scrooper/models"
)

// CanadeskAdapter handles canadesk/realtor-canada actor
type CanadeskAdapter struct {
	DaysBack int // set by handler before BuildInput
}

func (a *CanadeskAdapter) ActorID() string {
	return "canadesk~realtor-canada"
}

func (a *CanadeskAdapter) BuildInput(region config.Region, isIncremental bool) map[string]interface{} {
	// Coordinates format: "latMax,lngMin,latMin,lngMax"
	coordinates := fmt.Sprintf("%f,%f,%f,%f",
		region.LatMax, region.LngMin, region.LatMin, region.LngMax,
	)

	days := a.DaysBack
	if days == 0 {
		// fallback to old behavior if not set
		days = 30
		if isIncremental {
			days = 1
		}
	}

	return map[string]interface{}{
		"city":         extractCityName(region.GeoName),
		"country":      "Canada",
		"coordinates":  coordinates,
		"days":         days,
		"delay":        3,
		"frompage":     1,
		"listing_type": "for_sale",
		"bedrooms":     "1-0",
		"bathrooms":    "1-0",
		"process":      "sl",
		"zoom":         12,
		"proxy": map[string]interface{}{
			"useApifyProxy":     true,
			"apifyProxyGroups":  []string{"RESIDENTIAL"},
			"apifyProxyCountry": "CA",
		},
	}
}

func extractCityName(geoName string) string {
	// "Windsor, ON" -> "Windsor"
	for i, c := range geoName {
		if c == ',' {
			return geoName[:i]
		}
	}
	return geoName
}

func (a *CanadeskAdapter) ParseListing(data json.RawMessage) (models.RawListing, error) {
	var r canadeskListing
	if err := json.Unmarshal(data, &r); err != nil {
		return models.RawListing{}, err
	}

	beds, bedsPlus := parseBedrooms(r.Building.Bedrooms)

	listing := models.RawListing{
		ID:           r.ID,
		MLS:          r.MlsNumber,
		Address:      r.Property.Address.AddressText,
		City:         r.Property.Address.City,
		Province:     normalizeProvince(r.ProvinceName),
		PostalCode:   r.PostalCode,
		Price:        parsePrice(r.Property.Price),
		Beds:         beds,
		BedsPlus:     bedsPlus,
		Baths:        parseIntString(r.Building.BathroomTotal),
		SqFt:         parseIntString(r.Building.SizeInterior),
		PropertyType: r.Property.Type,
		URL:          "https://www.realtor.ca" + r.RelativeURLEn,
		Photos:       extractCanadeskPhotos(r.Property.Photo),
		Description:  r.PublicRemarks,
		Realtor:      extractCanadeskRealtor(r.Individual),
		Data:         data,
	}

	if listing.City == "" {
		listing.City = extractCity(r.Property.Address.AddressText)
	}

	return listing, nil
}

// FilterListings filters by city name in address (canadesk is fuzzy about location)
func (a *CanadeskAdapter) FilterListings(listings []models.RawListing, region config.Region) []models.RawListing {
	cityName := strings.ToLower(extractCityName(region.GeoName))
	var result []models.RawListing
	for _, l := range listings {
		if strings.Contains(strings.ToLower(l.Address), cityName) {
			result = append(result, l)
		}
	}
	return result
}

type canadeskListing struct {
	ID            string `json:"Id"`
	MlsNumber     string `json:"MlsNumber"`
	PublicRemarks string `json:"PublicRemarks"`
	RelativeURLEn string `json:"RelativeURLEn"`
	PostalCode    string `json:"PostalCode"`
	ProvinceName  string `json:"ProvinceName"`
	Property      struct {
		Price   string `json:"Price"`
		Type    string `json:"Type"`
		Address struct {
			AddressText string `json:"AddressText"`
			City        string `json:"City"`
			Province    string `json:"Province"`
		} `json:"Address"`
		Photo []struct {
			HighResPath string `json:"HighResPath"`
			LowResPath  string `json:"LowResPath"`
		} `json:"Photo"`
	} `json:"Property"`
	Building struct {
		Bedrooms      string `json:"Bedrooms"`
		BathroomTotal string `json:"BathroomTotal"`
		SizeInterior  string `json:"SizeInterior"`
	} `json:"Building"`
	Individual []struct {
		IndividualID int    `json:"IndividualID"`
		Name         string `json:"Name"`
		Photo        string `json:"Photo"`
		Phones       []struct {
			PhoneType   string `json:"PhoneType"`
			PhoneNumber string `json:"PhoneNumber"`
			AreaCode    string `json:"AreaCode"`
		} `json:"Phones"`
		Organization struct {
			OrganizationID int    `json:"OrganizationID"`
			Name           string `json:"Name"`
			Logo           string `json:"Logo"`
			Address        struct {
				AddressText string `json:"AddressText"`
			} `json:"Address"`
			Phones []struct {
				PhoneType   string `json:"PhoneType"`
				PhoneNumber string `json:"PhoneNumber"`
				AreaCode    string `json:"AreaCode"`
			} `json:"Phones"`
		} `json:"Organization"`
	} `json:"Individual"`
}

func extractCanadeskPhotos(photos []struct {
	HighResPath string `json:"HighResPath"`
	LowResPath  string `json:"LowResPath"`
}) []string {
	var urls []string
	for _, p := range photos {
		if p.HighResPath != "" {
			urls = append(urls, p.HighResPath)
		} else if p.LowResPath != "" {
			urls = append(urls, p.LowResPath)
		}
	}
	return urls
}

// normalizeProvince converts province name to 2-letter code
func normalizeProvince(name string) string {
	switch strings.ToLower(name) {
	case "ontario":
		return "ON"
	case "quebec", "québec":
		return "QC"
	case "british columbia":
		return "BC"
	case "alberta":
		return "AB"
	case "manitoba":
		return "MB"
	case "saskatchewan":
		return "SK"
	case "nova scotia":
		return "NS"
	case "new brunswick":
		return "NB"
	case "newfoundland and labrador", "newfoundland":
		return "NL"
	case "prince edward island":
		return "PE"
	case "northwest territories":
		return "NT"
	case "yukon":
		return "YT"
	case "nunavut":
		return "NU"
	}
	return name // Return as-is if already a code or unknown
}

func extractCanadeskRealtor(individuals []struct {
	IndividualID int    `json:"IndividualID"`
	Name         string `json:"Name"`
	Photo        string `json:"Photo"`
	Phones       []struct {
		PhoneType   string `json:"PhoneType"`
		PhoneNumber string `json:"PhoneNumber"`
		AreaCode    string `json:"AreaCode"`
	} `json:"Phones"`
	Organization struct {
		OrganizationID int    `json:"OrganizationID"`
		Name           string `json:"Name"`
		Logo           string `json:"Logo"`
		Address        struct {
			AddressText string `json:"AddressText"`
		} `json:"Address"`
		Phones []struct {
			PhoneType   string `json:"PhoneType"`
			PhoneNumber string `json:"PhoneNumber"`
			AreaCode    string `json:"AreaCode"`
		} `json:"Phones"`
	} `json:"Organization"`
}) *models.Realtor {
	if len(individuals) == 0 {
		return nil
	}

	firstOrg := individuals[0].Organization
	realtor := &models.Realtor{
		Company: models.RealtorCompany{
			ID:      firstOrg.OrganizationID,
			Name:    firstOrg.Name,
			Phone:   formatPhoneSlice(firstOrg.Phones),
			Address: firstOrg.Address.AddressText,
			Logo:    firstOrg.Logo,
		},
	}

	for _, ind := range individuals {
		agent := models.RealtorAgent{
			ID:    ind.IndividualID,
			Name:  ind.Name,
			Phone: formatPhoneSlice(ind.Phones),
			Photo: ind.Photo,
		}
		realtor.Agents = append(realtor.Agents, agent)
	}

	return realtor
}

func formatPhoneSlice(phones []struct {
	PhoneType   string `json:"PhoneType"`
	PhoneNumber string `json:"PhoneNumber"`
	AreaCode    string `json:"AreaCode"`
}) string {
	for _, p := range phones {
		if p.PhoneType == "Telephone" && p.AreaCode != "" && p.PhoneNumber != "" {
			return p.AreaCode + "-" + p.PhoneNumber
		}
	}
	if len(phones) > 0 && phones[0].AreaCode != "" && phones[0].PhoneNumber != "" {
		return phones[0].AreaCode + "-" + phones[0].PhoneNumber
	}
	return ""
}

func parseIntString(s string) int {
	var result int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result*10 + int(c-'0')
		} else if result > 0 {
			break
		}
	}
	return result
}

// parseBedrooms handles "3 + 1" format, returns (beds, bedsPlus)
func parseBedrooms(s string) (int, int) {
	// Look for " + " pattern
	parts := strings.Split(s, "+")
	if len(parts) == 2 {
		beds := parseIntString(strings.TrimSpace(parts[0]))
		plus := parseIntString(strings.TrimSpace(parts[1]))
		return beds, plus
	}
	return parseIntString(s), 0
}
