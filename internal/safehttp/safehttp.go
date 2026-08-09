// Package safehttp provides an HTTP client hardened against SSRF.
//
// It is intentionally dependency-free (stdlib only) so that low-level
// packages such as linkpreview and activitypub can import it without
// creating an import cycle with internal/api.
//
// The core protection is a dial-time guard: a custom net.Dialer.Control
// callback inspects the *actual* IP the connection is about to reach and
// rejects private / loopback / link-local / cloud-metadata targets. Because
// the check runs on the resolved address immediately before connect, it is
// immune to DNS-rebinding TOCTOU (validate-then-fetch) attacks that a
// hostname-based pre-check cannot stop. Redirects are bounded and every hop
// is re-dialed through the same guard, so a public host cannot 302 to an
// internal address.
package safehttp

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// ErrBlockedAddress is returned when a request targets a disallowed
// (private/internal) address or scheme.
var ErrBlockedAddress = errors.New("address blocked: refusing to connect to a private or internal host")

// privateNetworks lists CIDR ranges treated as private/internal. Requests
// resolving to any of these are refused.
var privateNetworks []*net.IPNet

func init() {
	cidrs := []string{
		"0.0.0.0/8",          // "this" network
		"10.0.0.0/8",         // RFC 1918
		"100.64.0.0/10",      // RFC 6598 carrier-grade NAT
		"127.0.0.0/8",        // loopback
		"169.254.0.0/16",     // link-local (incl. 169.254.169.254 cloud metadata)
		"172.16.0.0/12",      // RFC 1918
		"192.0.0.0/24",       // IETF protocol assignments
		"192.0.2.0/24",       // TEST-NET-1
		"192.168.0.0/16",     // RFC 1918
		"198.18.0.0/15",      // benchmarking
		"198.51.100.0/24",    // TEST-NET-2
		"203.0.113.0/24",     // TEST-NET-3
		"240.0.0.0/4",        // reserved
		"255.255.255.255/32", // broadcast
		"::1/128",            // IPv6 loopback
		"::/128",             // IPv6 unspecified
		"fc00::/7",           // IPv6 unique local
		"fe80::/10",          // IPv6 link-local
	}
	for _, cidr := range cidrs {
		if _, network, err := net.ParseCIDR(cidr); err == nil {
			privateNetworks = append(privateNetworks, network)
		}
	}
}

// IsBlockedIP reports whether ip is private/internal and must not be reached.
// IPv4-mapped IPv6 addresses (::ffff:127.0.0.1) are normalized first so they
// cannot slip past the IPv4 ranges.
func IsBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	for _, network := range privateNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// ValidateURL checks scheme (http/https only) and that any IP-literal host is
// not private. Hostnames are not resolved here — the dial-time guard handles
// resolution safely. Use this as a cheap up-front reject; the real protection
// is the client's dialer.
func ValidateURL(rawURL string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL must use http or https")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL has no host")
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return ErrBlockedAddress
	}
	if ip := net.ParseIP(host); ip != nil && IsBlockedIP(ip) {
		return ErrBlockedAddress
	}
	return nil
}

// guardedControl is a net.Dialer.Control callback that rejects connections to
// blocked IPs. address is "ip:port" with the IP already resolved.
func guardedControl(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return ErrBlockedAddress
	}
	ip := net.ParseIP(host)
	if ip == nil || IsBlockedIP(ip) {
		return ErrBlockedAddress
	}
	return nil
}

// Options configure a safe client.
type Options struct {
	Timeout      time.Duration // total request timeout (default 10s)
	MaxRedirects int           // redirect hops allowed (default 5)
}

// NewClient returns an *http.Client whose dialer refuses private/internal
// addresses and whose redirect policy re-applies the same guard on every hop.
func NewClient(opts Options) *http.Client {
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	if opts.MaxRedirects <= 0 {
		opts.MaxRedirects = 5
	}
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   guardedControl,
	}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{
		Timeout:   opts.Timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= opts.MaxRedirects {
				return fmt.Errorf("too many redirects")
			}
			// Re-validate scheme on each hop; the dialer validates the IP.
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return ErrBlockedAddress
			}
			return nil
		},
	}
}
