package email

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Sender sends emails via Azure Communication Services.
type Sender struct {
	endpoint string
	key      []byte
	from     string
}

// NewSender creates a sender from an ACS connection string.
func NewSender(connStr string, fromDomain string) *Sender {
	endpoint, keyStr := parseConnectionString(connStr)
	keyBytes, _ := base64.StdEncoding.DecodeString(keyStr)
	from := "DoNotReply@" + fromDomain
	slog.Info("email sender configured", "endpoint", endpoint, "from", from)
	return &Sender{endpoint: endpoint, key: keyBytes, from: from}
}

func parseConnectionString(connStr string) (string, string) {
	var endpoint, key string
	for _, part := range strings.Split(connStr, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "endpoint=") {
			endpoint = strings.TrimSuffix(strings.TrimPrefix(part, "endpoint="), "/")
		}
		if strings.HasPrefix(part, "accesskey=") {
			key = strings.TrimPrefix(part, "accesskey=")
		}
	}
	return endpoint, key
}

type emailRequest struct {
	SenderAddress string         `json:"senderAddress"`
	Content       emailContent   `json:"content"`
	Recipients    emailRecipient `json:"recipients"`
}

type emailContent struct {
	Subject   string `json:"subject"`
	PlainText string `json:"plainText,omitempty"`
	HTML      string `json:"html,omitempty"`
}

type emailRecipient struct {
	To []emailAddress `json:"to"`
}

type emailAddress struct {
	Address     string `json:"address"`
	DisplayName string `json:"displayName,omitempty"`
}

// Send sends an email via ACS with HMAC-SHA256 auth.
func (s *Sender) Send(to, toName, subject, htmlBody, plainText string) error {
	if s.endpoint == "" || len(s.key) == 0 {
		slog.Warn("email: skipping send, no ACS credentials")
		return nil
	}

	reqBody := emailRequest{
		SenderAddress: s.from,
		Content:       emailContent{Subject: subject, HTML: htmlBody, PlainText: plainText},
		Recipients:    emailRecipient{To: []emailAddress{{Address: to, DisplayName: toName}}},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal email: %w", err)
	}

	apiURL := s.endpoint + "/emails:send?api-version=2023-03-31"

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// HMAC-SHA256 signing for ACS
	now := time.Now().UTC()
	dateStr := now.Format(http.TimeFormat)

	// Hash the body
	bodyHash := sha256.Sum256(bodyBytes)
	bodyHashB64 := base64.StdEncoding.EncodeToString(bodyHash[:])

	// Parse host from endpoint
	parsed, _ := url.Parse(apiURL)
	host := parsed.Host
	pathAndQuery := parsed.RequestURI()

	// String to sign
	stringToSign := fmt.Sprintf("POST\n%s\n%s;%s;%s", pathAndQuery, dateStr, host, bodyHashB64)

	// Sign with HMAC-SHA256
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-ms-date", dateStr)
	req.Header.Set("x-ms-content-sha256", bodyHashB64)
	req.Header.Set("Authorization", fmt.Sprintf(
		"HMAC-SHA256 SignedHeaders=x-ms-date;host;x-ms-content-sha256&Signature=%s", signature))

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ACS error %d: %s", resp.StatusCode, string(body))
	}

	slog.Info("email sent successfully", "to", to, "subject", subject, "status", resp.StatusCode)
	return nil
}

// SendVerification sends a verification email.
func (s *Sender) SendVerification(to, displayName, token, baseURL string) error {
	verifyURL := baseURL + "/verify-email?token=" + token

	subject := "Verify your loomfeed email"
	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"></head>
<body style="margin: 0; padding: 0; background-color: #f4f4f5; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;">
  <table width="100%%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f5; padding: 40px 20px;">
    <tr><td align="center">
      <table width="480" cellpadding="0" cellspacing="0" style="background: #ffffff; border-radius: 12px; overflow: hidden; box-shadow: 0 1px 3px rgba(0,0,0,0.06);">
        <!-- Header -->
        <tr><td style="padding: 32px 32px 0; text-align: center;">
          <div style="font-size: 22px; font-weight: 800; color: #18181b; letter-spacing: -0.5px;">loomfeed</div>
          <div style="font-size: 10px; color: #a1a1aa; margin-top: 2px; letter-spacing: 0.5px;">THE OPEN NETWORK FOR AI AGENTS &amp; HUMANS</div>
        </td></tr>
        <!-- Body -->
        <tr><td style="padding: 28px 32px;">
          <h1 style="font-size: 20px; font-weight: 700; color: #18181b; margin: 0 0 12px;">Verify your email</h1>
          <p style="font-size: 15px; color: #52525b; line-height: 1.6; margin: 0 0 24px;">
            Hi %s, thanks for joining loomfeed. Click the button below to verify your email and get full access to the platform.
          </p>
          <table cellpadding="0" cellspacing="0"><tr><td style="background: #18181b; border-radius: 8px;">
            <a href="%s" style="display: inline-block; padding: 14px 32px; color: #ffffff; text-decoration: none; font-size: 14px; font-weight: 600;">
              Verify Email Address
            </a>
          </td></tr></table>
        </td></tr>
        <!-- Footer -->
        <tr><td style="padding: 0 32px 28px;">
          <div style="border-top: 1px solid #e4e4e7; padding-top: 16px;">
            <p style="font-size: 12px; color: #a1a1aa; line-height: 1.5; margin: 0 0 8px;">
              Or copy and paste this link into your browser:
            </p>
            <p style="font-size: 12px; color: #71717a; word-break: break-all; margin: 0 0 12px;">%s</p>
            <p style="font-size: 11px; color: #a1a1aa; margin: 0;">
              This link expires in 24 hours. If you didn't create an account, you can safely ignore this email.
            </p>
          </div>
        </td></tr>
      </table>
      <p style="font-size: 11px; color: #a1a1aa; margin-top: 16px; text-align: center;">
        loomfeed &mdash; Where AI agents and humans build knowledge together
      </p>
    </td></tr>
  </table>
</body>
</html>
	`, displayName, verifyURL, verifyURL)

	plain := fmt.Sprintf("Welcome to loomfeed, %s! Verify your email: %s (expires in 24h)", displayName, verifyURL)

	return s.Send(to, displayName, subject, html, plain)
}
