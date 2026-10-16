package graphserver

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"log/slog"
	"math/big"
	"net"
	netsmtp "net/smtp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/andrewheberle/graph-smtpd/pkg/sendmail"
	"github.com/emersion/go-smtp"
)

// logBuffer collects JSON log lines written by the server's goroutines.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.buf.Write(p)
}

// lines returns the logged lines with the given message.
func (l *logBuffer) lines(t *testing.T, msg string) []map[string]any {
	t.Helper()

	l.mu.Lock()
	defer l.mu.Unlock()

	var found []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(l.buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("log line is not JSON: %q", line)
		}
		if entry["msg"] == msg {
			found = append(found, entry)
		}
	}

	return found
}

// waitFor waits until n lines with the given message have been logged,
// since some, such as "disconnect", are logged after the client has gone.
func (l *logBuffer) waitFor(t *testing.T, msg string, n int) []map[string]any {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for {
		lines := l.lines(t, msg)
		if len(lines) >= n || time.Now().After(deadline) {
			return lines
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// startLoggedServer runs an SMTP server for mailer that logs to the returned
// buffer at debug level. With tlsConfig set it offers STARTTLS.
func startLoggedServer(t *testing.T, mailer Mailer, tlsConfig *tls.Config, opts ...BackendOption) (string, *logBuffer) {
	t.Helper()

	logs := &logBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	be, err := NewBackend(mailer, append(opts, WithLogger(logger))...)
	if err != nil {
		t.Fatalf("NewBackend() error = %v", err)
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not listen: %v", err)
	}
	s := smtp.NewServer(be)
	s.Domain = "localhost"
	s.TLSConfig = tlsConfig
	go func() { _ = s.Serve(l) }()
	t.Cleanup(func() { _ = s.Close() })

	return l.Addr().String(), logs
}

// selfSigned returns a TLS config with a new self-signed certificate for
// 127.0.0.1, and a pool that trusts it.
func selfSigned(t *testing.T) (*tls.Config, *x509.CertPool) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("could not generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("could not create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("could not parse certificate: %v", err)
	}

	roots := x509.NewCertPool()
	roots.AddCert(cert)

	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}, roots
}

// checkFields reports any field of line that differs from want.
func checkFields(t *testing.T, line map[string]any, want map[string]any) {
	t.Helper()

	for k, v := range want {
		if got := line[k]; got != v {
			t.Errorf("%s = %v (%T), want %v (%T); line: %v", k, got, got, v, v, line)
		}
	}
}

const loggedMessage = "From: alice@example.com\r\nTo: bob@example.com\r\nMessage-ID: <abc123@example.com>\r\nSubject: Private subject\r\n\r\nHello Bob.\r\n"

func TestLogSentMessage(t *testing.T) {
	addr, logs := startLoggedServer(t, &fakeMailer{}, nil)
	c := dial(t, addr)

	if err := send(c, "alice@example.com", []string{"bob@example.com"}, loggedMessage); err != nil {
		t.Fatalf("send() error = %v", err)
	}
	if err := c.Quit(); err != nil {
		t.Fatalf("Quit() error = %v", err)
	}

	connects := logs.waitFor(t, "connect", 1)
	messages := logs.waitFor(t, "message", 1)
	disconnects := logs.waitFor(t, "disconnect", 1)
	if len(connects) != 1 || len(messages) != 1 || len(disconnects) != 1 {
		t.Fatalf("got %d connect, %d message and %d disconnect lines, want one of each", len(connects), len(messages), len(disconnects))
	}

	conn, _ := connects[0]["conn"].(string)
	if conn == "" {
		t.Fatalf("connect line has no conn: %v", connects[0])
	}
	if client, _ := connects[0]["client"].(string); !strings.HasPrefix(client, "127.0.0.1:") {
		t.Errorf("client = %q, want 127.0.0.1:<port>", client)
	}
	checkFields(t, connects[0], map[string]any{"level": "INFO", "helo": "localhost", "tls": false})

	msg := messages[0]
	checkFields(t, msg, map[string]any{
		"level":      "INFO",
		"conn":       conn,
		"id":         conn + ".1",
		"from":       "alice@example.com",
		"message_id": "<abc123@example.com>",
		"status":     "sent",
		"reply":      "250 2.0.0 OK: queued",
		"helo":       "localhost",
		"tls":        false,
	})
	if rcpt, _ := msg["rcpt"].([]any); len(rcpt) != 1 || rcpt[0] != "bob@example.com" {
		t.Errorf("rcpt = %v, want [bob@example.com]", msg["rcpt"])
	}
	if size, _ := msg["size"].(float64); size < float64(len("Hello Bob.")) {
		t.Errorf("size = %v, want the message size", msg["size"])
	}
	if _, ok := msg["duration"]; !ok {
		t.Errorf("message line has no duration: %v", msg)
	}
	if _, ok := msg["reason"]; ok {
		t.Errorf("sent message has a reason: %v", msg)
	}
	if strings.Contains(logs.buf.String(), "Private subject") && len(logs.lines(t, "message details")) == 0 {
		t.Error("subject logged outside the debug details line")
	}
	for _, line := range messages {
		if _, ok := line["subject"]; ok {
			t.Errorf("message line includes the subject: %v", line)
		}
	}

	checkFields(t, disconnects[0], map[string]any{"conn": conn, "messages": float64(1), "sent": float64(1)})
}

func TestLogMessageOutcomes(t *testing.T) {
	tests := []struct {
		name       string
		mailer     *fakeMailer
		opts       []BackendOption
		data       string
		wantStatus string
		wantReply  string
		wantReason string
	}{
		{
			name:       "sender not allowed",
			mailer:     &fakeMailer{},
			opts:       []BackendOption{WithAllowedSenders([]string{"zoe@example.com"})},
			data:       testMessage,
			wantStatus: "rejected",
			wantReply:  "550 5.7.1 sender is not permitted",
			wantReason: "sender is not in the allowed senders",
		},
		{
			name:       "source not allowed",
			mailer:     &fakeMailer{},
			opts:       []BackendOption{WithAllowedSources([]string{"192.0.2.1"})},
			data:       testMessage,
			wantStatus: "rejected",
			wantReply:  "550 5.7.1 relay from this address is not permitted",
			wantReason: "client address is not in the allowed sources",
		},
		{
			name:       "temporary Graph failure",
			mailer:     &fakeMailer{err: errors.New("graph unavailable")},
			data:       testMessage,
			wantStatus: "deferred",
			wantReply:  "451 4.4.1 temporary failure delivering message, try again later",
			wantReason: "graph unavailable",
		},
		{
			name:       "Graph rejection",
			mailer:     &fakeMailer{err: &sendmail.RejectedError{StatusCode: 403, Code: "ErrorAccessDenied"}},
			data:       testMessage,
			wantStatus: "rejected",
			wantReply:  "550 5.7.1 delivery refused by Microsoft Graph (ErrorAccessDenied)",
			wantReason: "message rejected by Microsoft Graph",
		},
		{
			name:       "unparseable message",
			mailer:     &fakeMailer{},
			data:       "not a header line\r\n\r\nHello.\r\n",
			wantStatus: "rejected",
			wantReply:  "554 5.6.0 message could not be processed",
			wantReason: "malformed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, logs := startLoggedServer(t, tt.mailer, nil, tt.opts...)

			_ = send(dial(t, addr), "alice@example.com", []string{"bob@example.com"}, tt.data)

			messages := logs.waitFor(t, "message", 1)
			if len(messages) != 1 {
				t.Fatalf("got %d message lines, want 1", len(messages))
			}
			msg := messages[0]
			checkFields(t, msg, map[string]any{"level": "WARN", "status": tt.wantStatus, "from": "alice@example.com"})
			if _, ok := msg["rcpt"].([]any); !ok {
				t.Errorf("rcpt = %v, want a list", msg["rcpt"])
			}
			if reply, _ := msg["reply"].(string); !strings.HasPrefix(reply, tt.wantReply) {
				t.Errorf("reply = %q, want prefix %q", reply, tt.wantReply)
			}
			if reason, _ := msg["reason"].(string); !strings.Contains(reason, tt.wantReason) {
				t.Errorf("reason = %q, want it to contain %q", reason, tt.wantReason)
			}
			if errorLines := logs.lines(t, ""); len(errorLines) != 0 {
				t.Errorf("unexpected unnamed log lines: %v", errorLines)
			}
			if strings.Contains(logs.buf.String(), `"level":"ERROR"`) {
				t.Errorf("message refusal logged at ERROR: %s", logs.buf.String())
			}
		})
	}
}

func TestLogAbandonedMessage(t *testing.T) {
	addr, logs := startLoggedServer(t, &fakeMailer{}, nil)
	c := dial(t, addr)

	if err := c.Mail("alice@example.com"); err != nil {
		t.Fatalf("Mail() error = %v", err)
	}
	if err := c.Rcpt("bob@example.com"); err != nil {
		t.Fatalf("Rcpt() error = %v", err)
	}
	if err := c.Quit(); err != nil {
		t.Fatalf("Quit() error = %v", err)
	}

	messages := logs.waitFor(t, "message", 1)
	if len(messages) != 1 {
		t.Fatalf("got %d message lines, want 1", len(messages))
	}
	checkFields(t, messages[0], map[string]any{"level": "INFO", "status": "abandoned", "from": "alice@example.com"})

	disconnects := logs.waitFor(t, "disconnect", 1)
	if len(disconnects) != 1 {
		t.Fatalf("got %d disconnect lines, want 1", len(disconnects))
	}
	checkFields(t, disconnects[0], map[string]any{"messages": float64(1), "abandoned": float64(1)})
}

func TestLogNoMessageLineWithoutMessage(t *testing.T) {
	addr, logs := startLoggedServer(t, &fakeMailer{}, nil)
	c := dial(t, addr)

	if err := c.Hello("localhost"); err != nil {
		t.Fatalf("Hello() error = %v", err)
	}
	if err := c.Reset(); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}
	if err := c.Quit(); err != nil {
		t.Fatalf("Quit() error = %v", err)
	}

	if disconnects := logs.waitFor(t, "disconnect", 1); len(disconnects) != 1 {
		t.Fatalf("got %d disconnect lines, want 1", len(disconnects))
	}
	if messages := logs.lines(t, "message"); len(messages) != 0 {
		t.Errorf("got message lines without a message: %v", messages)
	}
}

func TestLogMessagesShareConnection(t *testing.T) {
	addr, logs := startLoggedServer(t, &fakeMailer{}, nil)
	c := dial(t, addr)

	for range 2 {
		if err := send(c, "alice@example.com", []string{"bob@example.com"}, testMessage); err != nil {
			t.Fatalf("send() error = %v", err)
		}
	}

	messages := logs.waitFor(t, "message", 2)
	if len(messages) != 2 {
		t.Fatalf("got %d message lines, want 2", len(messages))
	}
	conn := messages[0]["conn"]
	checkFields(t, messages[0], map[string]any{"id": conn.(string) + ".1"})
	checkFields(t, messages[1], map[string]any{"conn": conn, "id": conn.(string) + ".2"})
}

func TestLogStartTLS(t *testing.T) {
	tlsConfig, roots := selfSigned(t)
	addr, logs := startLoggedServer(t, &fakeMailer{}, tlsConfig)

	c, err := netsmtp.Dial(addr)
	if err != nil {
		t.Fatalf("could not dial: %v", err)
	}
	if err := c.StartTLS(&tls.Config{ServerName: "127.0.0.1", RootCAs: roots}); err != nil {
		t.Fatalf("StartTLS() error = %v", err)
	}
	if err := send(c, "alice@example.com", []string{"bob@example.com"}, testMessage); err != nil {
		t.Fatalf("send() error = %v", err)
	}
	if err := c.Quit(); err != nil {
		t.Fatalf("Quit() error = %v", err)
	}

	disconnects := logs.waitFor(t, "disconnect", 1)
	connects := logs.lines(t, "connect")
	upgrades := logs.lines(t, "starttls")
	messages := logs.lines(t, "message")
	if len(connects) != 1 || len(upgrades) != 1 || len(messages) != 1 || len(disconnects) != 1 {
		t.Fatalf("got %d connect, %d starttls, %d message and %d disconnect lines, want one of each",
			len(connects), len(upgrades), len(messages), len(disconnects))
	}

	conn := connects[0]["conn"]
	checkFields(t, connects[0], map[string]any{"tls": false})
	checkFields(t, upgrades[0], map[string]any{"conn": conn, "tls": true})
	if v, _ := upgrades[0]["tls_version"].(string); !strings.HasPrefix(v, "TLS 1.") {
		t.Errorf("tls_version = %q, want a TLS version", v)
	}
	checkFields(t, messages[0], map[string]any{"conn": conn, "tls": true, "status": "sent"})
	checkFields(t, disconnects[0], map[string]any{"conn": conn, "tls": true, "sent": float64(1)})
}
