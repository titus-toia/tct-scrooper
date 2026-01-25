package scraper

// Shared helper functions for Apify adapters

func extractPostalFromAddress(address string) string {
	// Address format: "Street|City, Province PostalCode"
	if len(address) < 6 {
		return ""
	}
	// Canadian postal codes: A1A1A1 or A1A 1A1 (last 6-7 chars)
	parts := splitAddress(address)
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		// Extract postal code pattern from end
		if len(last) >= 6 {
			// Try to find postal code at end (format: X#X #X# or X#X#X#)
			candidate := last[len(last)-7:]
			if isCanadianPostal(candidate) {
				return normalizePostal(candidate)
			}
			candidate = last[len(last)-6:]
			if isCanadianPostal(candidate) {
				return normalizePostal(candidate)
			}
		}
	}
	return ""
}

func isCanadianPostal(s string) bool {
	// Remove spaces and check pattern A#A#A#
	clean := ""
	for _, c := range s {
		if c != ' ' {
			clean += string(c)
		}
	}
	if len(clean) != 6 {
		return false
	}
	// Check pattern: letter digit letter digit letter digit
	for i, c := range clean {
		if i%2 == 0 {
			if c < 'A' || c > 'Z' {
				return false
			}
		} else {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

func normalizePostal(s string) string {
	clean := ""
	for _, c := range s {
		if c != ' ' {
			clean += string(c)
		}
	}
	return clean
}
