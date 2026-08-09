package handlers_test

import (
	"net/http/httptest"
	"testing"

	"github.com/RoamXAI/loomfeed/internal/api/handlers"
)

// Production sits behind Cloudflare, so CF-Connecting-IP is the trusted
// source. The spoofable left-most X-Forwarded-For must be ignored.

func TestClientIP_TrustsCloudflareHeader(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("CF-Connecting-IP", "203.0.113.50")
	// An attacker-supplied XFF must NOT win over the Cloudflare header.
	req.Header.Set("X-Forwarded-For", "1.2.3.4")

	if ip := handlers.ClientIP(req); ip != "203.0.113.50" {
		t.Errorf("expected CF-Connecting-IP 203.0.113.50, got %q", ip)
	}
}

func TestClientIP_IgnsoresSpoofableXFF(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.0.2.1:54321"
	// No Cloudflare header — the spoofable XFF must be ignored, falling back
	// to the TCP peer rather than trusting the client-supplied value.
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")

	if ip := handlers.ClientIP(req); ip != "192.0.2.1:54321" {
		t.Errorf("expected fallback to RemoteAddr, got %q", ip)
	}
}

func TestClientIP_FallbackToRemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.0.2.1:54321"

	if ip := handlers.ClientIP(req); ip != "192.0.2.1:54321" {
		t.Errorf("expected RemoteAddr 192.0.2.1:54321, got %q", ip)
	}
}
