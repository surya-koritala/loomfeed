package api

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

var (
	slugRegex  = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,48}[a-z0-9]$`)
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
)

// ValidateEmail checks that the email address is well-formed.
func ValidateEmail(email string) error {
	if !emailRegex.MatchString(email) {
		return fmt.Errorf("invalid email format")
	}
	return nil
}

// ValidateSlug checks that a community slug is 3-50 lowercase alphanumeric chars,
// optionally separated by hyphens or underscores.
func ValidateSlug(slug string) error {
	if !slugRegex.MatchString(slug) {
		return fmt.Errorf("slug must be 3-50 chars, lowercase alphanumeric with hyphens/underscores")
	}
	return nil
}

// ValidateURL checks that rawURL is a valid http or https URL.
func ValidateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL must use http or https")
	}
	return nil
}

// privateNetworks is the list of CIDR ranges considered private/internal.
// Webhook URLs pointing to these ranges are rejected to prevent SSRF attacks.
var privateNetworks []*net.IPNet

func init() {
	cidrs := []string{
		"10.0.0.0/8",      // RFC 1918
		"172.16.0.0/12",   // RFC 1918
		"192.168.0.0/16",  // RFC 1918
		"127.0.0.0/8",     // loopback
		"169.254.0.0/16",  // link-local
		"0.0.0.0/8",       // "this" network
		"::1/128",         // IPv6 loopback
		"fc00::/7",        // IPv6 unique local
		"fe80::/10",       // IPv6 link-local
	}
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil {
			privateNetworks = append(privateNetworks, network)
		}
	}
}

// isPrivateIP checks whether the given IP falls within any private/internal range.
func isPrivateIP(ip net.IP) bool {
	for _, network := range privateNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// ValidateWebhookURL checks that rawURL is a valid http/https URL that does not
// point to private or loopback addresses (SSRF prevention). It performs DNS
// resolution on hostnames to catch private IPs behind public-looking domains.
func ValidateWebhookURL(rawURL string) error {
	if err := ValidateURL(rawURL); err != nil {
		return err
	}
	u, _ := url.Parse(rawURL)
	host := u.Hostname()

	// Block "localhost" explicitly (including subdomains like foo.localhost)
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return fmt.Errorf("webhook URL cannot point to private addresses")
	}

	// Check if host is an IP literal
	ip := net.ParseIP(host)
	if ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("webhook URL cannot point to private addresses")
		}
		return nil
	}

	// Host is a hostname — resolve it and check all returned IPs
	ips, err := net.LookupIP(host)
	if err != nil {
		// Can't resolve — allow it (will fail on actual delivery)
		return nil
	}
	for _, resolved := range ips {
		if isPrivateIP(resolved) {
			return fmt.Errorf("webhook URL cannot point to private addresses")
		}
	}
	return nil
}

// ValidateLength checks that the trimmed length of value is within [min, max].
func ValidateLength(field, value string, min, max int) error {
	l := len(strings.TrimSpace(value))
	if l < min {
		return fmt.Errorf("%s must be at least %d characters", field, min)
	}
	if l > max {
		return fmt.Errorf("%s exceeds %d character limit", field, max)
	}
	return nil
}
