package safehttp

import (
	"net"
	"testing"
)

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "127.5.5.5", "::1",
		"169.254.169.254", // cloud metadata
		"10.0.0.5", "172.16.0.1", "192.168.1.1",
		"100.64.0.1",          // CGNAT
		"0.0.0.0",             // unspecified
		"::ffff:127.0.0.1",    // IPv4-mapped loopback
		"::ffff:169.254.169.254",
		"fe80::1",  // link-local
		"fc00::1",  // unique local
	}
	for _, s := range blocked {
		if ip := net.ParseIP(s); !IsBlockedIP(ip) {
			t.Errorf("expected %s to be BLOCKED", s)
		}
	}

	allowed := []string{
		"8.8.8.8", "1.1.1.1", "93.184.216.34", "2606:2800:220:1::1",
	}
	for _, s := range allowed {
		if ip := net.ParseIP(s); IsBlockedIP(ip) {
			t.Errorf("expected %s to be ALLOWED", s)
		}
	}

	if !IsBlockedIP(nil) {
		t.Error("nil IP must be treated as blocked")
	}
}

func TestValidateURL(t *testing.T) {
	bad := []string{
		"",
		"ftp://example.com",
		"file:///etc/passwd",
		"javascript:alert(1)",
		"http://localhost/x",
		"http://foo.localhost/x",
		"http://127.0.0.1/x",
		"http://169.254.169.254/latest/meta-data/",
		"http://[::1]/x",
		"https://10.0.0.1/internal",
	}
	for _, u := range bad {
		if err := ValidateURL(u); err == nil {
			t.Errorf("expected %q to be rejected", u)
		}
	}

	good := []string{
		"http://example.com",
		"https://example.com/path?q=1",
		"https://8.8.8.8/x",
	}
	for _, u := range good {
		if err := ValidateURL(u); err != nil {
			t.Errorf("expected %q to be allowed, got %v", u, err)
		}
	}
}
