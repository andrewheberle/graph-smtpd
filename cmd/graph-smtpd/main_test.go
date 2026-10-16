package main

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andrewheberle/graph-smtpd/pkg/graphserver"
	"github.com/emersion/go-smtp"
	"github.com/spf13/pflag"
	"golang.org/x/crypto/bcrypt"
)

func TestLoadConfigTimeout(t *testing.T) {
	tests := []struct {
		name string
		env  string
		args []string
		want time.Duration
	}{
		{"default", "", nil, graphserver.DefaultSendTimeout},
		{"from env", "45s", nil, 45 * time.Second},
		{"flag wins over env", "45s", []string{"--timeout", "1m"}, time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv("SMTPD_TIMEOUT", tt.env)
			}

			f := pflag.NewFlagSet("config", pflag.ContinueOnError)
			f.String("config", "", "")
			f.Duration("timeout", graphserver.DefaultSendTimeout, "")
			if err := f.Parse(tt.args); err != nil {
				t.Fatalf("could not parse flags: %v", err)
			}

			cfg, err := loadConfig(f)
			if err != nil {
				t.Fatalf("loadConfig() error = %v", err)
			}
			if got := cfg.Duration("timeout"); got != tt.want {
				t.Errorf("timeout = %v, want %v", got, tt.want)
			}
		})
	}
}

// nopBackend accepts sessions that do nothing.
type nopBackend struct{}

func (nopBackend) NewSession(c *smtp.Conn) (smtp.Session, error) { return nopSession{}, nil }

type nopSession struct{}

func (nopSession) Mail(string, *smtp.MailOptions) error { return nil }
func (nopSession) Rcpt(string, *smtp.RcptOptions) error { return nil }
func (nopSession) Data(io.Reader) error                 { return nil }
func (nopSession) Reset()                               {}
func (nopSession) Logout() error                        { return nil }

func TestNewServerClosesIdleConnections(t *testing.T) {
	f := pflag.NewFlagSet("config", pflag.ContinueOnError)
	f.String("config", "", "")
	f.Duration("conntimeout", 5*time.Minute, "")
	if err := f.Parse([]string{"--conntimeout", "100ms"}); err != nil {
		t.Fatalf("could not parse flags: %v", err)
	}
	cfg, err := loadConfig(f)
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not listen: %v", err)
	}
	s := newServer(nopBackend{}, cfg, slog.New(slog.DiscardHandler))
	go func() { _ = s.Serve(l) }()
	t.Cleanup(func() { _ = s.Close() })

	conn, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatalf("could not dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// read the greeting, then stay idle until the server gives up
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("could not set deadline: %v", err)
	}
	r := bufio.NewReader(conn)
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatalf("could not read greeting: %v", err)
	}
	for {
		if _, err := r.ReadString('\n'); err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				t.Fatal("idle connection was not closed by the server")
			}
			return
		}
	}
}

func TestNewServerLogsSMTPErrors(t *testing.T) {
	f := pflag.NewFlagSet("config", pflag.ContinueOnError)
	f.String("config", "", "")
	if err := f.Parse(nil); err != nil {
		t.Fatalf("could not parse flags: %v", err)
	}
	cfg, err := loadConfig(f)
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}

	var buf bytes.Buffer
	s := newServer(nopBackend{}, cfg, slog.New(slog.NewJSONHandler(&buf, nil)))
	s.ErrorLog.Printf("handshake failed")

	if !strings.Contains(buf.String(), `"msg":"smtp server error"`) || !strings.Contains(buf.String(), "handshake failed") {
		t.Errorf("SMTP server error not logged: %q", buf.String())
	}
}

func TestLoadUsers(t *testing.T) {
	dir := t.TempDir()
	usersFile := filepath.Join(dir, "users")
	hashed, err := bcrypt.GenerateFromPassword([]byte("scanner-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("could not hash password: %v", err)
	}
	if err := os.WriteFile(usersFile, []byte("scanner:"+string(hashed)+"\n"), 0o600); err != nil {
		t.Fatalf("could not write users file: %v", err)
	}

	tests := []struct {
		name      string
		args      []string
		wantUsers int
		wantErr   string
	}{
		{"no users file", nil, 0, ""},
		{"users file with TLS", []string{"--users", usersFile, "--cert", "tls.crt", "--key", "tls.key"}, 1, ""},
		{"users file without TLS", []string{"--users", usersFile}, 0, "needs --cert and --key"},
		{"missing users file", []string{"--users", filepath.Join(dir, "missing"), "--cert", "tls.crt", "--key", "tls.key"}, 0, "could not open"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := pflag.NewFlagSet("config", pflag.ContinueOnError)
			f.String("config", "", "")
			f.String("users", "", "")
			f.String("cert", "", "")
			f.String("key", "", "")
			if err := f.Parse(tt.args); err != nil {
				t.Fatalf("could not parse flags: %v", err)
			}
			cfg, err := loadConfig(f)
			if err != nil {
				t.Fatalf("loadConfig() error = %v", err)
			}

			users, err := loadUsers(cfg)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("loadUsers() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("loadUsers() error = %v", err)
			}
			got := 0
			if users != nil {
				got = users.Len()
			}
			if got != tt.wantUsers {
				t.Errorf("users = %d, want %d", got, tt.wantUsers)
			}
		})
	}
}
