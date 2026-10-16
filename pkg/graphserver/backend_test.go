package graphserver

import (
	"context"
	"errors"
	"net"
	netsmtp "net/smtp"
	"slices"
	"sync"
	"testing"

	"github.com/andrewheberle/graph-smtpd/pkg/sendmail"
	"github.com/emersion/go-smtp"
	"github.com/prometheus/client_golang/prometheus"
)

// fakeMailer records each Message it is asked to send, failing with err when
// it is set.
type fakeMailer struct {
	mu   sync.Mutex
	sent []*sendmail.Message
	err  error
}

func (f *fakeMailer) Send(ctx context.Context, m *sendmail.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, m)

	return nil
}

func (f *fakeMailer) messages() []*sendmail.Message {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.sent)
}

// startServer runs an SMTP server for mailer on a loopback port and returns
// its address and metrics registry.
func startServer(t *testing.T, mailer Mailer, opts ...BackendOption) (string, *prometheus.Registry) {
	t.Helper()

	reg := prometheus.NewRegistry()
	be, err := NewBackend(mailer, append(opts, WithPrometheusRegistry(reg))...)
	if err != nil {
		t.Fatalf("NewBackend() error = %v", err)
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not listen: %v", err)
	}

	s := smtp.NewServer(be)
	s.Domain = "localhost"
	go func() { _ = s.Serve(l) }()
	t.Cleanup(func() { _ = s.Close() })

	return l.Addr().String(), reg
}

func dial(t *testing.T, addr string) *netsmtp.Client {
	t.Helper()

	c, err := netsmtp.Dial(addr)
	if err != nil {
		t.Fatalf("could not dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	return c
}

// send sends one message over c, returning the first error from the SMTP
// conversation.
func send(c *netsmtp.Client, from string, rcpt []string, data string) error {
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, r := range rcpt {
		if err := c.Rcpt(r); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(data)); err != nil {
		return err
	}

	return w.Close()
}

const testMessage = "From: alice@example.com\r\nTo: bob@example.com\r\nSubject: Hello\r\n\r\nHello Bob.\r\n"

// counter returns the value of the named counter in reg.
func counter(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("could not gather metrics: %v", err)
	}
	for _, f := range families {
		if f.GetName() == name {
			return f.GetMetric()[0].GetCounter().GetValue()
		}
	}
	t.Fatalf("metric %q not found", name)

	return 0
}

// recipients returns the to and bcc addresses of m, as logged.
func recipients(m *sendmail.Message) (to, bcc []string) {
	for _, a := range m.LogValue().Group() {
		switch a.Key {
		case "to":
			to = a.Value.Any().([]string)
		case "bcc":
			bcc = a.Value.Any().([]string)
		}
	}

	return to, bcc
}

func TestNewBackendNilMailer(t *testing.T) {
	if _, err := NewBackend(nil); err == nil {
		t.Errorf("NewBackend(nil) error = nil, want error")
	}
}

func TestBackendDeliversMessage(t *testing.T) {
	mailer := &fakeMailer{}
	addr, _ := startServer(t, mailer)

	if err := send(dial(t, addr), "alice@example.com", []string{"bob@example.com"}, testMessage); err != nil {
		t.Fatalf("send() error = %v", err)
	}

	sent := mailer.messages()
	if len(sent) != 1 {
		t.Fatalf("sent = %d messages, want 1", len(sent))
	}
	if got := sent[0].From(); got != "alice@example.com" {
		t.Errorf("From() = %q, want %q", got, "alice@example.com")
	}
	if to, _ := recipients(sent[0]); !slices.Equal(to, []string{"bob@example.com"}) {
		t.Errorf("to = %v, want [bob@example.com]", to)
	}
}

func TestBackendMessagesInOneSessionAreIndependent(t *testing.T) {
	mailer := &fakeMailer{}
	addr, _ := startServer(t, mailer)
	c := dial(t, addr)

	if err := send(c, "alice@example.com", []string{"bob@example.com"}, testMessage); err != nil {
		t.Fatalf("first send() error = %v", err)
	}
	if err := send(c, "alice@example.com", []string{"carol@example.com"}, testMessage); err != nil {
		t.Fatalf("second send() error = %v", err)
	}

	sent := mailer.messages()
	if len(sent) != 2 {
		t.Fatalf("sent = %d messages, want 2", len(sent))
	}
	// carol is not in the To header, so she is delivered as Bcc, and bob
	// is not an envelope recipient of the second message
	to, bcc := recipients(sent[1])
	if len(to) != 0 || !slices.Equal(bcc, []string{"carol@example.com"}) {
		t.Errorf("second message to = %v, bcc = %v, want to = [], bcc = [carol@example.com]", to, bcc)
	}
}

func TestBackendSenderNotAllowed(t *testing.T) {
	mailer := &fakeMailer{}
	addr, reg := startServer(t, mailer, WithAllowedSenders([]string{"zoe@example.com"}))

	if err := send(dial(t, addr), "alice@example.com", []string{"bob@example.com"}, testMessage); err == nil {
		t.Errorf("send() error = nil, want sender rejected")
	}
	if n := len(mailer.messages()); n != 0 {
		t.Errorf("sent = %d messages, want 0", n)
	}
	if got := counter(t, reg, "graph_smtpd_email_denied_total"); got != 1 {
		t.Errorf("denied = %v, want 1", got)
	}
}

func TestBackendSources(t *testing.T) {
	tests := []struct {
		name       string
		sources    []string
		wantSent   int
		wantDenied bool
	}{
		{"source allowed", []string{"127.0.0.1"}, 1, false},
		{"source not allowed", []string{"192.0.2.1"}, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mailer := &fakeMailer{}
			addr, reg := startServer(t, mailer, WithAllowedSources(tt.sources))

			// a denied source may be rejected at connect or at MAIL
			if c, err := netsmtp.Dial(addr); err == nil {
				_ = send(c, "alice@example.com", []string{"bob@example.com"}, testMessage)
				_ = c.Close()
			}

			if n := len(mailer.messages()); n != tt.wantSent {
				t.Errorf("sent = %d messages, want %d", n, tt.wantSent)
			}
			// clients that fall back from EHLO to HELO are denied twice, so
			// only check whether any denial was counted
			if got := counter(t, reg, "graph_smtpd_email_denied_total"); (got > 0) != tt.wantDenied {
				t.Errorf("denied = %v, want denied %v", got, tt.wantDenied)
			}
		})
	}
}

func TestBackendMailerError(t *testing.T) {
	mailer := &fakeMailer{err: errors.New("graph unavailable")}
	addr, reg := startServer(t, mailer)

	if err := send(dial(t, addr), "alice@example.com", []string{"bob@example.com"}, testMessage); err == nil {
		t.Errorf("send() error = nil, want DATA rejected")
	}
	if got := counter(t, reg, "graph_smtpd_email_errors_total"); got != 1 {
		t.Errorf("errors = %v, want 1", got)
	}
}
