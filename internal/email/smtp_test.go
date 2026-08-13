package email

import (
	"bufio"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/config"
)

func TestConfiguredSenderUsesSMTP(t *testing.T) {
	host, port, received := startSMTPServer(t, "mailer", "secret")
	sender := NewConfiguredSender(config.EmailConfig{
		SMTPHost:     host,
		SMTPPort:     port,
		SMTPUsername: "mailer",
		SMTPPassword: "secret",
		SMTPFrom:     "Loomfeed <hello@loomfeed.test>",
	})
	if sender == nil {
		t.Fatal("expected configured SMTP sender")
	}

	if err := sender.Send("reader@example.com", "Reader", "Weekly digest", "<strong>HTML copy</strong>", "Plain copy"); err != nil {
		t.Fatalf("send SMTP email: %v", err)
	}

	select {
	case message := <-received:
		for _, want := range []string{
			`From: "Loomfeed" <hello@loomfeed.test>`,
			`To: "Reader" <reader@example.com>`,
			"Subject: Weekly digest",
			"Content-Type: multipart/alternative",
			"Plain copy",
			"<strong>HTML copy</strong>",
		} {
			if !strings.Contains(message, want) {
				t.Errorf("message missing %q\n%s", want, message)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SMTP server did not receive the message")
	}
}

func TestIdempotentSMTPSendUsesStableMessageID(t *testing.T) {
	host, port, received := startSMTPServer(t, "mailer", "secret")
	sender := NewSMTPSender(host, port, "mailer", "secret", "Loomfeed <hello@loomfeed.test>")

	if err := sender.SendIdempotent(
		"d65d25a5-7d37-43c9-a12d-4f2e4faf1b83",
		time.Date(2026, time.August, 13, 9, 0, 0, 0, time.UTC),
		"reader@example.com", "Reader", "Weekly digest", "<strong>HTML</strong>", "Plain",
	); err != nil {
		t.Fatalf("send idempotent SMTP email: %v", err)
	}

	select {
	case message := <-received:
		if !strings.Contains(message, "Message-ID: <d65d25a5-7d37-43c9-a12d-4f2e4faf1b83@loomfeed.test>") {
			t.Fatalf("message missing stable delivery ID\n%s", message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SMTP server did not receive the message")
	}
}

func TestIdempotentACSSendUsesRepeatabilityHeaders(t *testing.T) {
	var headers http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header.Clone()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	sender := &Sender{backend: &acsBackend{endpoint: server.URL, key: []byte("secret"), from: "sender@example.com"}}
	firstSent := time.Date(2026, time.August, 13, 9, 0, 0, 0, time.UTC)
	deliveryID := "d65d25a5-7d37-43c9-a12d-4f2e4faf1b83"

	if err := sender.SendIdempotent(
		deliveryID, firstSent,
		"reader@example.com", "Reader", "Weekly digest", "<strong>HTML</strong>", "Plain",
	); err != nil {
		t.Fatalf("send idempotent ACS email: %v", err)
	}
	if got := headers.Get("Operation-Id"); got != deliveryID {
		t.Fatalf("Operation-Id=%q, want %q", got, deliveryID)
	}
	if got := headers.Get("Repeatability-Request-Id"); got != deliveryID {
		t.Fatalf("Repeatability-Request-Id=%q, want %q", got, deliveryID)
	}
	if got := headers.Get("Repeatability-First-Sent"); got != firstSent.Format(http.TimeFormat) {
		t.Fatalf("Repeatability-First-Sent=%q, want %q", got, firstSent.Format(http.TimeFormat))
	}
}

func TestConfiguredSenderFallsBackToACS(t *testing.T) {
	sender := NewConfiguredSender(config.EmailConfig{
		ACSConnectionString: "endpoint=https://example.communication.azure.com;accesskey=YWJj",
		ACSEmailDomain:      "example.test",
	})
	if sender == nil {
		t.Fatal("expected configured ACS sender")
	}
}

func TestConfiguredSenderDisabledWithoutCredentials(t *testing.T) {
	if sender := NewConfiguredSender(config.EmailConfig{}); sender != nil {
		t.Fatal("expected no sender without SMTP or ACS configuration")
	}
}

func startSMTPServer(t *testing.T, username, password string) (string, string, <-chan string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	received := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
		writeSMTPLine(rw, "220 localhost ESMTP")

		var message strings.Builder
		inData := false
		for {
			line, err := rw.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
			if inData {
				if line == "." {
					inData = false
					received <- message.String()
					writeSMTPLine(rw, "250 queued")
					continue
				}
				message.WriteString(line)
				message.WriteString("\r\n")
				continue
			}

			command, argument, _ := strings.Cut(line, " ")
			switch strings.ToUpper(command) {
			case "EHLO":
				_, _ = rw.WriteString("250-localhost\r\n250 AUTH PLAIN\r\n")
				_ = rw.Flush()
			case "AUTH":
				parts := strings.Fields(argument)
				if len(parts) != 2 || strings.ToUpper(parts[0]) != "PLAIN" {
					writeSMTPLine(rw, "535 authentication failed")
					continue
				}
				credentials, _ := base64.StdEncoding.DecodeString(parts[1])
				if string(credentials) != "\x00"+username+"\x00"+password {
					writeSMTPLine(rw, "535 authentication failed")
					continue
				}
				writeSMTPLine(rw, "235 authenticated")
			case "MAIL", "RCPT":
				writeSMTPLine(rw, "250 ok")
			case "DATA":
				inData = true
				writeSMTPLine(rw, "354 end with <CRLF>.<CRLF>")
			case "QUIT":
				writeSMTPLine(rw, "221 bye")
				return
			default:
				writeSMTPLine(rw, "250 ok")
			}
		}
	}()

	host, port, _ := net.SplitHostPort(listener.Addr().String())
	return host, port, received
}

func writeSMTPLine(rw *bufio.ReadWriter, line string) {
	_, _ = rw.WriteString(line + "\r\n")
	_ = rw.Flush()
}
