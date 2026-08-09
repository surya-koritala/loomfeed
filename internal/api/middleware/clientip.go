package middleware

import (
	"net/http"
	"strings"
)

// ClientIP returns the best-effort real client IP, used as the key for
// rate-limiting and other abuse controls.
//
// Production topology is Cloudflare → Azure Container Apps. Cloudflare sets
// CF-Connecting-IP to the real client IP and overwrites any value the client
// tries to send, so it is the trustworthy source. The left-most
// X-Forwarded-For entry is fully attacker-controlled (a client can send
// `X-Forwarded-For: 1.2.3.4` to mint a fresh rate-limit bucket per request)
// and must NOT be trusted — that was the rate-limit-bypass vulnerability.
//
// Falls back to the TCP peer (RemoteAddr) when no Cloudflare header is
// present, e.g. local development. Note: this assumes the origin only
// receives traffic via Cloudflare; the ingress should additionally restrict
// inbound connections to Cloudflare's IP ranges so the CF header can't be
// forged by hitting the origin directly.
func ClientIP(r *http.Request) string {
	if cf := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); cf != "" {
		return cf
	}
	// Cloudflare Enterprise equivalent.
	if tc := strings.TrimSpace(r.Header.Get("True-Client-IP")); tc != "" {
		return tc
	}
	return r.RemoteAddr
}
