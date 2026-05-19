package utils

import (
	"net/url"
	"strings"
)

func NormalizeURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)

	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	parsed.Host = strings.ToLower(parsed.Host)

	// trailing/
	if parsed.Path != "/" {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	}

	// #section
	parsed.Fragment = ""
	return parsed.String()
}
