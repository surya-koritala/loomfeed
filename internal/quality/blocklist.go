package quality

import (
	"net"
	"strings"
)

// blockedDomains contains domains that are never valid sources.
var blockedDomains = map[string]string{
	// RFC 2606 reserved
	"example.com":     "reserved domain",
	"example.org":     "reserved domain",
	"example.net":     "reserved domain",
	"test.com":        "reserved domain",
	"test.org":        "reserved domain",
	"invalid":         "reserved domain",
	"localhost":        "local address",

	// Placeholder/fake
	"placeholder.com":     "placeholder domain",
	"via.placeholder.com": "placeholder domain",
	"placehold.it":        "placeholder domain",
	"fakeurl.com":         "fake domain",
	"fakesource.com":      "fake domain",
	"notarealsite.com":    "fake domain",
	"domain.com":          "generic placeholder",
	"website.com":         "generic placeholder",
	"yoursite.com":        "generic placeholder",
	"mysite.com":          "generic placeholder",
	"sample.com":          "generic placeholder",
	"foo.com":             "generic placeholder",
	"bar.com":             "generic placeholder",

	// Content farms / low quality
	"blogspot.com": "low quality source",
}

// blockedSuffixes catches subdomains of blocked domains.
var blockedSuffixes = []string{
	".example.com",
	".example.org",
	".example.net",
	".test.com",
	".localhost",
}

// IsBlocked returns true if the domain is on the blocklist, along with the reason.
func IsBlocked(domain string) (bool, string) {
	domain = strings.ToLower(strings.TrimSpace(domain))

	// Direct match
	if reason, ok := blockedDomains[domain]; ok {
		return true, reason
	}

	// Suffix match (subdomains of blocked domains)
	for _, suffix := range blockedSuffixes {
		if strings.HasSuffix(domain, suffix) {
			return true, "subdomain of blocked domain"
		}
	}

	// Loopback / private IP addresses
	if ip := net.ParseIP(domain); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() {
			return true, "private/loopback IP"
		}
	}

	// Common loopback strings
	if domain == "127.0.0.1" || domain == "0.0.0.0" || strings.HasPrefix(domain, "192.168.") || strings.HasPrefix(domain, "10.") {
		return true, "private/loopback IP"
	}

	return false, ""
}
