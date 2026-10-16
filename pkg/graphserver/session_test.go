package graphserver

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestSessionResetClearsMessageState(t *testing.T) {
	var buf bytes.Buffer

	s := &Session{
		logger:     slog.New(slog.NewJSONHandler(&buf, nil)),
		policy:     mustRelayPolicy(t, []string{"zoe@example.com"}, nil),
		sendErrors: prometheus.NewCounter(prometheus.CounterOpts{Name: "errors"}),
		sendDenied: prometheus.NewCounter(prometheus.CounterOpts{Name: "denied"}),
	}

	// a sender not in the allowed list is recorded and then rejected
	if err := s.Mail("alice@example.com", nil); err == nil {
		t.Fatalf("Mail() error = nil, want error")
	}
	if err := s.Rcpt("bob@example.com", nil); err != nil {
		t.Fatalf("Rcpt() error = %v", err)
	}

	s.Reset()

	if s.from != "" {
		t.Errorf("from = %q, want empty", s.from)
	}
	if len(s.rcpt) != 0 {
		t.Errorf("rcpt = %v, want empty", s.rcpt)
	}
	if len(s.errors) != 0 {
		t.Errorf("errors = %v, want empty", s.errors)
	}

	buf.Reset()
	if err := s.Rcpt("carol@example.com", nil); err != nil {
		t.Fatalf("Rcpt() error = %v", err)
	}
	s.Reset()
	if strings.Contains(buf.String(), "alice@example.com") {
		t.Errorf("next message logged with previous message fields: %s", buf.String())
	}
}
