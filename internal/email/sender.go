package email

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/surya-koritala/loomfeed/internal/config"
)

type deliveryBackend interface {
	Send(to, toName, subject, htmlBody, plainText string) error
}

type idempotentDeliveryBackend interface {
	SendIdempotent(deliveryID string, firstSent time.Time, to, toName, subject, htmlBody, plainText string) error
}

// Sender exposes one email interface over the configured delivery backend.
type Sender struct {
	backend deliveryBackend
}

type acsBackend struct {
	endpoint string
	key      []byte
	from     string
}

type smtpBackend struct {
	host     string
	port     string
	username string
	password string
	from     string
}

// NewSender creates a sender from an ACS connection string.
// Kept for compatibility with callers that explicitly construct ACS.
func NewSender(connStr string, fromDomain string) *Sender {
	endpoint, keyStr := parseConnectionString(connStr)
	keyBytes, _ := base64.StdEncoding.DecodeString(keyStr)
	from := "DoNotReply@" + fromDomain
	slog.Info("email sender configured", "endpoint", endpoint, "from", from)
	return &Sender{backend: &acsBackend{endpoint: endpoint, key: keyBytes, from: from}}
}

// NewSMTPSender creates a sender for standard SMTP. The transport upgrades
// with STARTTLS whenever the server advertises it and supports optional PLAIN
// authentication through SMTP_USERNAME/SMTP_PASSWORD.
func NewSMTPSender(host, port, username, password, from string) *Sender {
	slog.Info("SMTP email sender configured", "host", host, "port", port, "from", from)
	return &Sender{backend: &smtpBackend{
		host: host, port: port, username: username, password: password, from: from,
	}}
}

// NewConfiguredSender selects SMTP when SMTP_HOST is configured, otherwise
// preserving the existing ACS backend. A nil result means email is disabled.
func NewConfiguredSender(cfg config.EmailConfig) *Sender {
	if cfg.SMTPHost != "" {
		return NewSMTPSender(
			cfg.SMTPHost,
			cfg.SMTPPort,
			cfg.SMTPUsername,
			cfg.SMTPPassword,
			cfg.SMTPFrom,
		)
	}
	if cfg.ACSConnectionString != "" {
		return NewSender(cfg.ACSConnectionString, cfg.ACSEmailDomain)
	}
	return nil
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

// Send delegates to the configured backend.
func (s *Sender) Send(to, toName, subject, htmlBody, plainText string) error {
	if s == nil || s.backend == nil {
		return fmt.Errorf("email sender is not configured")
	}
	return s.backend.Send(to, toName, subject, htmlBody, plainText)
}

// SendIdempotent supplies a stable delivery identity to backends that can
// suppress or correlate retries. It falls back to Send for other backends.
func (s *Sender) SendIdempotent(
	deliveryID string,
	firstSent time.Time,
	to, toName, subject, htmlBody, plainText string,
) error {
	if s == nil || s.backend == nil {
		return fmt.Errorf("email sender is not configured")
	}
	if strings.ContainsAny(deliveryID, "\r\n") {
		return fmt.Errorf("email delivery ID must not contain line breaks")
	}
	if backend, ok := s.backend.(idempotentDeliveryBackend); ok {
		return backend.SendIdempotent(deliveryID, firstSent, to, toName, subject, htmlBody, plainText)
	}
	return s.backend.Send(to, toName, subject, htmlBody, plainText)
}

// Send sends an email via ACS with HMAC-SHA256 auth.
func (s *acsBackend) Send(to, toName, subject, htmlBody, plainText string) error {
	return s.send("", time.Time{}, to, toName, subject, htmlBody, plainText)
}

func (s *acsBackend) SendIdempotent(
	deliveryID string,
	firstSent time.Time,
	to, toName, subject, htmlBody, plainText string,
) error {
	return s.send(deliveryID, firstSent, to, toName, subject, htmlBody, plainText)
}

func (s *acsBackend) send(
	deliveryID string,
	firstSent time.Time,
	to, toName, subject, htmlBody, plainText string,
) error {
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
	if deliveryID != "" {
		req.Header.Set("Operation-Id", deliveryID)
		req.Header.Set("Repeatability-Request-Id", deliveryID)
		req.Header.Set("Repeatability-First-Sent", firstSent.UTC().Format(http.TimeFormat))
	}

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

// Send submits a multipart plain-text + HTML message over SMTP.
func (s *smtpBackend) Send(to, toName, subject, htmlBody, plainText string) error {
	return s.send("", to, toName, subject, htmlBody, plainText)
}

func (s *smtpBackend) SendIdempotent(
	deliveryID string,
	_ time.Time,
	to, toName, subject, htmlBody, plainText string,
) error {
	return s.send(deliveryID, to, toName, subject, htmlBody, plainText)
}

func (s *smtpBackend) send(deliveryID, to, toName, subject, htmlBody, plainText string) error {
	if s.host == "" || s.port == "" || s.from == "" {
		return fmt.Errorf("SMTP sender is incomplete")
	}
	if strings.ContainsAny(subject+toName, "\r\n") {
		return fmt.Errorf("email headers must not contain line breaks")
	}

	fromAddress, err := mail.ParseAddress(s.from)
	if err != nil {
		return fmt.Errorf("parse SMTP_FROM: %w", err)
	}
	toAddress, err := mail.ParseAddress(to)
	if err != nil {
		return fmt.Errorf("parse recipient: %w", err)
	}
	if toName != "" {
		toAddress.Name = toName
	}

	messageID := ""
	if deliveryID != "" {
		domain := "loomfeed.local"
		if _, fromDomain, ok := strings.Cut(fromAddress.Address, "@"); ok && fromDomain != "" {
			domain = fromDomain
		}
		messageID = fmt.Sprintf("<%s@%s>", deliveryID, domain)
	}
	message, err := buildSMTPMessage(*fromAddress, *toAddress, subject, plainText, htmlBody, messageID)
	if err != nil {
		return err
	}

	serverAddress := net.JoinHostPort(s.host, s.port)
	conn, err := net.DialTimeout("tcp", serverAddress, 15*time.Second)
	if err != nil {
		return fmt.Errorf("connect SMTP server: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("start SMTP client: %w", err)
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("start SMTP TLS: %w", err)
		}
	}
	if s.username != "" {
		if ok, _ := client.Extension("AUTH"); !ok {
			return fmt.Errorf("SMTP server does not advertise authentication")
		}
		auth := smtp.PlainAuth("", s.username, s.password, s.host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authenticate SMTP: %w", err)
		}
	}
	if err := client.Mail(fromAddress.Address); err != nil {
		return fmt.Errorf("set SMTP sender: %w", err)
	}
	if err := client.Rcpt(toAddress.Address); err != nil {
		return fmt.Errorf("set SMTP recipient: %w", err)
	}
	body, err := client.Data()
	if err != nil {
		return fmt.Errorf("begin SMTP message: %w", err)
	}
	if _, err := body.Write(message); err != nil {
		_ = body.Close()
		return fmt.Errorf("write SMTP message: %w", err)
	}
	if err := body.Close(); err != nil {
		return fmt.Errorf("finish SMTP message: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("quit SMTP session: %w", err)
	}

	slog.Info("email sent successfully", "provider", "smtp", "to", toAddress.Address, "subject", subject)
	return nil
}

func buildSMTPMessage(from, to mail.Address, subject, plainText, htmlBody string, messageID ...string) ([]byte, error) {
	var body bytes.Buffer
	boundary := multipart.NewWriter(&body)

	_, _ = fmt.Fprintf(&body, "From: %s\r\n", from.String())
	_, _ = fmt.Fprintf(&body, "To: %s\r\n", to.String())
	_, _ = fmt.Fprintf(&body, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	if len(messageID) > 0 && messageID[0] != "" {
		_, _ = fmt.Fprintf(&body, "Message-ID: %s\r\n", messageID[0])
	}
	_, _ = fmt.Fprint(&body, "MIME-Version: 1.0\r\n")
	_, _ = fmt.Fprintf(&body, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary.Boundary())

	writePart := func(contentType, content string) error {
		header := textproto.MIMEHeader{}
		header.Set("Content-Type", contentType+`; charset="UTF-8"`)
		header.Set("Content-Transfer-Encoding", "quoted-printable")
		part, err := boundary.CreatePart(header)
		if err != nil {
			return err
		}
		encoded := quotedprintable.NewWriter(part)
		if _, err := encoded.Write([]byte(content)); err != nil {
			_ = encoded.Close()
			return err
		}
		return encoded.Close()
	}

	if err := writePart("text/plain", plainText); err != nil {
		return nil, fmt.Errorf("encode SMTP plain-text body: %w", err)
	}
	if err := writePart("text/html", htmlBody); err != nil {
		return nil, fmt.Errorf("encode SMTP HTML body: %w", err)
	}
	if err := boundary.Close(); err != nil {
		return nil, fmt.Errorf("finish SMTP MIME body: %w", err)
	}
	return body.Bytes(), nil
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
