package api_test

import (
	"testing"

	"github.com/RoamXAI/loomfeed/internal/api"
)

func TestValidateWebhookURL_AllowsPublicURL(t *testing.T) {
	urls := []string{
		"https://hooks.example.com/webhook",
		"https://api.slack.com/hooks/abc",
		"http://webhook.site/test-123",
	}
	for _, u := range urls {
		if err := api.ValidateWebhookURL(u); err != nil {
			t.Errorf("expected %q to be allowed, got error: %v", u, err)
		}
	}
}

func TestValidateWebhookURL_BlocksPrivateIPs(t *testing.T) {
	urls := []string{
		"http://127.0.0.1/callback",
		"http://127.0.0.2:8080/hook",
		"http://10.0.0.1/webhook",
		"http://10.255.255.255/webhook",
		"http://172.16.0.1/webhook",
		"http://172.31.255.255/webhook",
		"http://192.168.0.1/webhook",
		"http://192.168.255.255/webhook",
		"http://169.254.1.1/webhook",
		"http://0.0.0.0/webhook",
		"http://localhost/webhook",
		"http://foo.localhost/webhook",
		"http://[::1]/webhook",
	}
	for _, u := range urls {
		if err := api.ValidateWebhookURL(u); err == nil {
			t.Errorf("expected %q to be blocked, but it was allowed", u)
		}
	}
}

func TestValidateWebhookURL_RejectsNonHTTP(t *testing.T) {
	urls := []string{
		"ftp://example.com/hook",
		"file:///etc/passwd",
		"javascript:alert(1)",
	}
	for _, u := range urls {
		if err := api.ValidateWebhookURL(u); err == nil {
			t.Errorf("expected %q to be rejected, but it was allowed", u)
		}
	}
}

func TestValidateWebhookURL_Blocks172Range(t *testing.T) {
	// Ensure the full 172.16.0.0/12 range is blocked (172.16.x.x through 172.31.x.x)
	blocked := []string{
		"http://172.16.0.1/hook",
		"http://172.20.0.1/hook",
		"http://172.31.255.255/hook",
	}
	for _, u := range blocked {
		if err := api.ValidateWebhookURL(u); err == nil {
			t.Errorf("expected %q to be blocked (private 172.16.0.0/12), but it was allowed", u)
		}
	}

	// 172.32.x.x is NOT private
	allowed := "http://172.32.0.1/hook"
	if err := api.ValidateWebhookURL(allowed); err != nil {
		t.Errorf("expected %q to be allowed (outside 172.16.0.0/12), got error: %v", allowed, err)
	}
}
