package activitypub

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// HTTP Signatures — the dialect used by Mastodon et al. (draft-cavage-
// http-signatures-12 with digest+date+host+request-target). Just enough
// to sign outbound POSTs and verify incoming ones.
//
// Canonical signing string joins the chosen headers as "name: value"
// lines separated by "\n". For POSTs we always sign:
//   (request-target) host date digest
// This matches Mastodon's default and is the broadest-compatible set.

// parsePrivateKey accepts a PEM-encoded RSA private key (either
// PKCS#1 "RSA PRIVATE KEY" or PKCS#8 "PRIVATE KEY") and returns the
// rsa.PrivateKey.
func parsePrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("no PEM block")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rk, ok := key.(*rsa.PrivateKey); ok {
			return rk, nil
		}
		return nil, fmt.Errorf("pkcs8 key is not rsa")
	}
	return nil, fmt.Errorf("unsupported private key format")
}

// parsePublicKey accepts a PEM-encoded public key (PKIX).
func parsePublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("no PEM block in public key")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		// Some servers serve PKCS#1. Try that too.
		if rk, pkcs1Err := x509.ParsePKCS1PublicKey(block.Bytes); pkcs1Err == nil {
			return rk, nil
		}
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	rk, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("key is not rsa")
	}
	return rk, nil
}

// sha256Digest computes SHA-256("=SHA-256=" style) base64 digest of a
// body — the format expected in the `Digest` request header.
func sha256Digest(body []byte) string {
	sum := sha256.Sum256(body)
	return "SHA-256=" + base64.StdEncoding.EncodeToString(sum[:])
}

// SignRequest signs an *http.Request in place with the given actor
// keyID (the full URI of the actor's publicKey, e.g.
// https://loomfeed.com/users/alice#main-key) and a PEM private key.
// Mutates the request: sets Date, Digest, Host, Signature headers.
func SignRequest(req *http.Request, keyID, privateKeyPEM string, body []byte) error {
	priv, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return err
	}

	req.Header.Set("Host", req.URL.Host)
	if req.Header.Get("Date") == "" {
		req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	}
	if body != nil && req.Method != http.MethodGet {
		req.Header.Set("Digest", sha256Digest(body))
	}

	headers := []string{"(request-target)", "host", "date"}
	if req.Header.Get("Digest") != "" {
		headers = append(headers, "digest")
	}

	signingStr := buildSigningString(req, headers)
	sum := sha256.Sum256([]byte(signingStr))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, sum[:])
	if err != nil {
		return fmt.Errorf("sign: %w", err)
	}

	header := fmt.Sprintf(
		`keyId="%s",algorithm="rsa-sha256",headers="%s",signature="%s"`,
		keyID, strings.Join(headers, " "), base64.StdEncoding.EncodeToString(sig),
	)
	req.Header.Set("Signature", header)
	return nil
}

// buildSigningString produces the canonical string described by
// draft-cavage: "name: value" joined by "\n", with request-target
// special-cased to "method path".
func buildSigningString(req *http.Request, headers []string) string {
	lines := make([]string, 0, len(headers))
	for _, h := range headers {
		lh := strings.ToLower(h)
		var v string
		if lh == "(request-target)" {
			path := req.URL.RequestURI()
			v = strings.ToLower(req.Method) + " " + path
		} else if lh == "host" {
			v = req.URL.Host
		} else {
			v = req.Header.Get(h)
		}
		lines = append(lines, lh+": "+v)
	}
	return strings.Join(lines, "\n")
}

// SignAttestation produces a base64 rsa-sha256 signature over the
// canonical string "issuer|issuedAt|score|scale". Used to embed
// a verifiable trust-score claim on actor documents — see
// docs/FEDIVERSE_TRUST.md for the wire format.
func SignAttestation(issuer, issuedAt, scale string, score float64, privateKeyPEM string) (string, error) {
	priv, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return "", err
	}
	payload := fmt.Sprintf("%s|%s|%.4f|%s", issuer, issuedAt, score, scale)
	sum := sha256.Sum256([]byte(payload))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, sum[:])
	if err != nil {
		return "", fmt.Errorf("sign attestation: %w", err)
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// VerifyAttestation is the inverse. Given the four fields and the
// signature, plus a publicKey, returns nil on valid signature.
func VerifyAttestation(issuer, issuedAt, scale string, score float64, signatureB64 string, publicKeyPEM string) error {
	pub, err := parsePublicKey(publicKeyPEM)
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return fmt.Errorf("decode attestation signature: %w", err)
	}
	payload := fmt.Sprintf("%s|%s|%.4f|%s", issuer, issuedAt, score, scale)
	sum := sha256.Sum256([]byte(payload))
	return rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig)
}

// parseSignatureHeader pulls the quoted params out of a Signature
// header. No attempt at robustness beyond Mastodon-compatible input.
func parseSignatureHeader(h string) map[string]string {
	out := map[string]string{}
	re := regexp.MustCompile(`(\w+)="([^"]*)"`)
	for _, m := range re.FindAllStringSubmatch(h, -1) {
		out[m[1]] = m[2]
	}
	return out
}

// VerifyRequest checks an incoming signed request against the public
// key at keyID. Fetches the key via resolveKey and validates algorithm,
// digest, and signature. Returns the resolved keyID on success so the
// caller can tie the request to an actor.
func VerifyRequest(req *http.Request, body []byte, resolveKey func(keyID string) (*rsa.PublicKey, error)) (string, error) {
	sigHeader := req.Header.Get("Signature")
	if sigHeader == "" {
		return "", fmt.Errorf("no Signature header")
	}
	params := parseSignatureHeader(sigHeader)
	keyID := params["keyId"]
	alg := params["algorithm"]
	hdrs := params["headers"]
	sigB64 := params["signature"]
	if keyID == "" || sigB64 == "" {
		return "", fmt.Errorf("missing keyId or signature")
	}
	// Accept only rsa-sha256 / hs2019. Mastodon defaults to rsa-sha256.
	if alg != "" && alg != "rsa-sha256" && alg != "hs2019" {
		return "", fmt.Errorf("unsupported algorithm: %s", alg)
	}
	if hdrs == "" {
		hdrs = "(created)"
	}

	headers := strings.Fields(hdrs)

	// Verify Digest if claimed in header list.
	for _, h := range headers {
		if strings.ToLower(h) == "digest" {
			want := sha256Digest(body)
			if req.Header.Get("Digest") != want {
				return "", fmt.Errorf("digest mismatch")
			}
		}
	}

	signingStr := buildSigningString(req, headers)
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return "", fmt.Errorf("decode signature: %w", err)
	}
	pub, err := resolveKey(keyID)
	if err != nil {
		return "", fmt.Errorf("resolve key: %w", err)
	}
	sum := sha256.Sum256([]byte(signingStr))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		return "", fmt.Errorf("verify: %w", err)
	}
	return keyID, nil
}

