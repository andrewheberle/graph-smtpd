package graphserver

import (
	"crypto/tls"
	"errors"
	netsmtp "net/smtp"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// loginAuth is a client for the LOGIN SASL mechanism, which net/smtp lacks.
type loginAuth struct {
	username, password string
}

func (a loginAuth) Start(*netsmtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", nil, nil
}

func (a loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch string(fromServer) {
	case "Username:":
		return []byte(a.username), nil
	case "Password:":
		return []byte(a.password), nil
	default:
		return nil, errors.New("unexpected challenge: " + string(fromServer))
	}
}

// withAuthDelay sets the wait after a failed login, so tests stay fast.
func withAuthDelay(d time.Duration) BackendOption {
	return func(b *Backend) {
		b.authDelay = d
	}
}

// authServer runs a server with STARTTLS and the user scanner, whose
// password is scanner-password, and returns a TLS client config for it.
func authServer(t *testing.T, opts ...BackendOption) (string, *logBuffer, *prometheus.Registry, *tls.Config) {
	t.Helper()

	tlsConfig, roots := selfSigned(t)
	reg := prometheus.NewRegistry()
	users := mustUsers(t, "scanner:"+hash(t, "scanner-password"))
	opts = append([]BackendOption{WithUsers(users), WithPrometheusRegistry(reg), withAuthDelay(0)}, opts...)
	addr, logs := startLoggedServer(t, &fakeMailer{}, tlsConfig, opts...)

	return addr, logs, reg, &tls.Config{ServerName: "127.0.0.1", RootCAs: roots}
}

// dialTLS connects to addr and upgrades with STARTTLS.
func dialTLS(t *testing.T, addr string, clientTLS *tls.Config) *netsmtp.Client {
	t.Helper()

	c := dial(t, addr)
	if err := c.StartTLS(clientTLS); err != nil {
		t.Fatalf("StartTLS() error = %v", err)
	}

	return c
}

func TestAuthNotOfferedWithoutTLS(t *testing.T) {
	addr, _, _, _ := authServer(t)
	c := dial(t, addr)

	if err := c.Hello("localhost"); err != nil {
		t.Fatalf("Hello() error = %v", err)
	}
	if ok, _ := c.Extension("AUTH"); ok {
		t.Error("AUTH offered before STARTTLS")
	}

	// and a client that tries anyway is told TLS is required
	err := c.Auth(netsmtp.PlainAuth("", "scanner", "scanner-password", "127.0.0.1"))
	if got := replyCode(err); got != 523 {
		t.Errorf("Auth() before STARTTLS: reply code = %d (%v), want 523", got, err)
	}
}

func TestAuthNotOfferedWithoutUsers(t *testing.T) {
	tlsConfig, roots := selfSigned(t)
	addr, _ := startLoggedServer(t, &fakeMailer{}, tlsConfig)
	c := dialTLS(t, addr, &tls.Config{ServerName: "127.0.0.1", RootCAs: roots})

	if ok, _ := c.Extension("AUTH"); ok {
		t.Error("AUTH offered with no users configured")
	}
}

func TestAuthOfferedOverTLS(t *testing.T) {
	addr, _, _, clientTLS := authServer(t)
	c := dialTLS(t, addr, clientTLS)

	ok, mechs := c.Extension("AUTH")
	if !ok || mechs != "PLAIN LOGIN" {
		t.Errorf("AUTH extension = %v %q, want PLAIN LOGIN", ok, mechs)
	}
}

func TestAuthLetsClientRelay(t *testing.T) {
	tests := []struct {
		name string
		auth netsmtp.Auth
		mech string
	}{
		{"PLAIN", netsmtp.PlainAuth("", "scanner", "scanner-password", "127.0.0.1"), "PLAIN"},
		{"LOGIN", loginAuth{"scanner", "scanner-password"}, "LOGIN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// this client's address is not an allowed source, so only logging
			// in lets it relay
			addr, logs, _, clientTLS := authServer(t, WithAllowedSources([]string{"192.0.2.1"}))
			c := dialTLS(t, addr, clientTLS)

			if err := c.Auth(tt.auth); err != nil {
				t.Fatalf("Auth() error = %v", err)
			}
			if err := send(c, "alice@example.com", []string{"bob@example.com"}, testMessage); err != nil {
				t.Fatalf("send() error = %v", err)
			}

			auths := logs.waitFor(t, "auth", 1)
			if len(auths) != 1 {
				t.Fatalf("got %d auth lines, want 1", len(auths))
			}
			checkFields(t, auths[0], map[string]any{"level": "INFO", "user": "scanner", "mechanism": tt.mech, "tls": true})

			messages := logs.waitFor(t, "message", 1)
			if len(messages) != 1 {
				t.Fatalf("got %d message lines, want 1", len(messages))
			}
			checkFields(t, messages[0], map[string]any{"status": "sent", "user": "scanner"})
		})
	}
}

func TestAuthFailure(t *testing.T) {
	tests := []struct {
		name string
		auth netsmtp.Auth
	}{
		{"wrong password", netsmtp.PlainAuth("", "scanner", "wrong-password", "127.0.0.1")},
		{"unknown user", netsmtp.PlainAuth("", "nobody", "scanner-password", "127.0.0.1")},
		{"PLAIN acting as another user", netsmtp.PlainAuth("printer", "scanner", "scanner-password", "127.0.0.1")},
		{"LOGIN wrong password", loginAuth{"scanner", "wrong-password"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, logs, reg, clientTLS := authServer(t)
			c := dialTLS(t, addr, clientTLS)

			err := c.Auth(tt.auth)
			if got := replyCode(err); got != 535 {
				t.Fatalf("Auth() reply code = %d (%v), want 535", got, err)
			}
			if got := counter(t, reg, "graph_smtpd_auth_failures_total"); got != 1 {
				t.Errorf("auth failures = %v, want 1", got)
			}

			failures := logs.waitFor(t, "auth failed", 1)
			if len(failures) != 1 {
				t.Fatalf("got %d auth failed lines, want 1", len(failures))
			}
			checkFields(t, failures[0], map[string]any{"level": "WARN"})
			if _, ok := failures[0]["username"]; !ok {
				t.Errorf("auth failed line has no username: %v", failures[0])
			}
			if strings.Contains(logs.buf.String(), "password\"") || strings.Contains(logs.buf.String(), "wrong-password") || strings.Contains(logs.buf.String(), "scanner-password") {
				t.Errorf("password logged: %s", logs.buf.String())
			}

			// net/smtp hangs up after a failed login; a client that is not
			// logged in, with no allowed sources, may not relay
			err = send(dialTLS(t, addr, clientTLS), "alice@example.com", []string{"bob@example.com"}, testMessage)
			if got := replyCode(err); got != 550 {
				t.Errorf("send() after failed login: reply code = %d (%v), want 550", got, err)
			}
		})
	}
}

func TestAuthFailureIsDelayed(t *testing.T) {
	addr, _, _, clientTLS := authServer(t, withAuthDelay(300*time.Millisecond))
	c := dialTLS(t, addr, clientTLS)

	start := time.Now()
	_ = c.Auth(netsmtp.PlainAuth("", "scanner", "wrong-password", "127.0.0.1"))
	if elapsed := time.Since(start); elapsed < 300*time.Millisecond {
		t.Errorf("failed login replied after %v, want at least 300ms", elapsed)
	}
}

func TestAuthRelayRules(t *testing.T) {
	tests := []struct {
		name       string
		sources    []string
		wantReply  int
		wantReason string
	}{
		{"no allowed sources requires login", nil, 550, "not authenticated"},
		{"unlisted source requires login", []string{"192.0.2.1"}, 550, "not authenticated"},
		{"allowed source may relay without login", []string{"127.0.0.1"}, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, logs, _, clientTLS := authServer(t, WithAllowedSources(tt.sources))
			c := dialTLS(t, addr, clientTLS)

			err := send(c, "alice@example.com", []string{"bob@example.com"}, testMessage)
			if got := replyCode(err); got != tt.wantReply {
				t.Fatalf("send() reply code = %d (%v), want %d", got, err, tt.wantReply)
			}
			if tt.wantReason != "" {
				messages := logs.waitFor(t, "message", 1)
				if len(messages) != 1 {
					t.Fatalf("got %d message lines, want 1", len(messages))
				}
				if reason, _ := messages[0]["reason"].(string); !strings.Contains(reason, tt.wantReason) {
					t.Errorf("reason = %q, want it to contain %q", reason, tt.wantReason)
				}
			}
		})
	}
}
