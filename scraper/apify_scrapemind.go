package scraper

import (
	"encoding/json"
	"fmt"
	"net/url"

	"tct_scrooper/config"
	"tct_scrooper/models"
)

// ScrapemindAdapter handles scrapemind/realtor-ca-scraper actor
type ScrapemindAdapter struct{}

func (a *ScrapemindAdapter) ActorID() string {
	return "scrapemind~realtor-ca-scraper"
}

// FilterListings returns listings as-is (scrapemind respects coordinates)
func (a *ScrapemindAdapter) FilterListings(listings []models.RawListing, region config.Region) []models.RawListing {
	return listings
}

func (a *ScrapemindAdapter) BuildInput(region config.Region, isIncremental bool) map[string]interface{} {
	searchURL := fmt.Sprintf(
		"https://www.realtor.ca/map#view=list&CurrentPage=1&Sort=6-D&GeoIds=%s&GeoName=%s&PropertyTypeGroupID=1&PropertySearchTypeId=1&Currency=CAD",
		region.GeoID, url.QueryEscape(region.GeoName),
	)

	return map[string]interface{}{
		"startUrls": []map[string]string{
			{"url": searchURL},
		},
		"proxyConfiguration": map[string]interface{}{
			"useApifyProxy":     true,
			"apifyProxyGroups":  []string{"RESIDENTIAL"},
			"apifyProxyCountry": "CA",
		},
		"getDetails":      true,
		"numberOfWorkers": 1,
		"simplifyOutput":  false,
	}
}

func (a *ScrapemindAdapter) ParseListing(data json.RawMessage) (models.RawListing, error) {
	var r scrapemindListing
	if err := json.Unmarshal(data, &r); err != nil {
		return models.RawListing{}, err
	}

	beds, bedsPlus := parseBedrooms(r.Building.Bedrooms)

	listing := models.RawListing{
		ID:           r.ID,
		MLS:          r.MlsNumber,
		Address:      r.Property.Address.AddressText,
		City:         r.Property.Address.City,
		PostalCode:   extractPostalFromAddress(r.Property.Address.AddressText),
		Price:        parsePrice(r.Property.Price),
		Beds:         beds,
		BedsPlus:     bedsPlus,
		Baths:        parseIntString(r.Building.BathroomTotal),
		SqFt:         parseIntString(r.Building.SizeInterior),
		PropertyType: r.Property.Type,
		URL:          "https://www.realtor.ca" + r.RelativeURLEn,
		Photos:       extractScrapemindPhotos(r.Property.Photo),
		Description:  r.PublicRemarks,
		Realtor:      extractScrapemindRealtor(r.Individual),
		Data:         data,
	}

	if listing.City == "" {
		listing.City = extractCity(r.Property.Address.AddressText)
	}

	return listing, nil
}

// scrapemindListing mirrors the Realtor.ca API response structure
type scrapemindListing struct {
	ID            string `json:"Id"`
	MlsNumber     string `json:"MlsNumber"`
	PublicRemarks string `json:"PublicRemarks"`
	RelativeURLEn string `json:"RelativeURLEn"`
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

func extractScrapemindPhotos(photos []struct {
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

func extractScrapemindRealtor(individuals []struct {
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
			Phone:   formatScrapemindPhone(firstOrg.Phones),
			Address: firstOrg.Address.AddressText,
		},
	}

	for _, ind := range individuals {
		agent := models.RealtorAgent{
			ID:    ind.IndividualID,
			Name:  ind.Name,
			Phone: formatScrapemindPhone(ind.Phones),
			Photo: ind.Photo,
		}
		realtor.Agents = append(realtor.Agents, agent)
	}

	return realtor
}

func formatScrapemindPhone(phones []struct {
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
