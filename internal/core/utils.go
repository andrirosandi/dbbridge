package core

import (
	"regexp"
	"strings"
)

// Slugify converts a string to a URL-safe slug
func Slugify(s string) string {
	// Lowercase
	s = strings.ToLower(s)

	// Replace spaces with dashes
	s = strings.ReplaceAll(s, " ", "-")

	// Remove non-alphanumeric characters (except dashes)
	reg := regexp.MustCompile("[^a-z0-9-]+")
	s = reg.ReplaceAllString(s, "")

	// Collapse multiple dashes
	regDash := regexp.MustCompile("-+")
	s = regDash.ReplaceAllString(s, "-")

	// Trim dashes
	s = strings.Trim(s, "-")

	return s
}
