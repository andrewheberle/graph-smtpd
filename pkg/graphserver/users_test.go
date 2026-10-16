package graphserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// hash returns a fast, minimum cost bcrypt hash of password for tests.
func hash(t *testing.T, password string) string {
	t.Helper()

	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("could not hash password: %v", err)
	}

	return string(h)
}

// mustUsers parses a users file built from lines.
func mustUsers(t *testing.T, lines ...string) *Users {
	t.Helper()

	users, err := ParseUsers(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatalf("ParseUsers() error = %v", err)
	}

	return users
}

func TestParseUsers(t *testing.T) {
	users := mustUsers(t,
		"# devices that relay through graph-smtpd",
		"",
		"scanner:"+hash(t, "scanner-password"),
		"  printer:"+strings.Replace(hash(t, "printer-password"), "$2a$", "$2y$", 1)+"  ",
	)

	if users.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", users.Len())
	}

	tests := []struct {
		username, password string
		want               bool
	}{
		{"scanner", "scanner-password", true},
		{"printer", "printer-password", true},
		{"scanner", "printer-password", false},
		{"scanner", "", false},
		{"Scanner", "scanner-password", false},
		{"nobody", "scanner-password", false},
	}
	for _, tt := range tests {
		if got := users.authenticate(tt.username, tt.password); got != tt.want {
			t.Errorf("authenticate(%q, %q) = %v, want %v", tt.username, tt.password, got, tt.want)
		}
	}
}

func TestParseUsersErrors(t *testing.T) {
	valid := hash(t, "password")

	tests := []struct {
		name    string
		line    string
		wantErr string
	}{
		{"plain text password", "scanner:hunter2-plaintext", "not a bcrypt hash"},
		{"md5 htpasswd hash", "scanner:$apr1$abc$abcdefghijklmnopqrstuv", "not a bcrypt hash"},
		{"truncated bcrypt hash", "scanner:" + valid[:20], "not a bcrypt hash"},
		{"no separator", "scanner", "want username:hash"},
		{"empty username", ":" + valid, "want username:hash"},
		{"username with space", "scan ner:" + valid, "want username:hash"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseUsers(strings.NewReader("# header\n" + tt.line + "\n"))
			if err == nil {
				t.Fatal("ParseUsers() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "line 2") {
				t.Errorf("ParseUsers() error = %q, want it to name line 2 and contain %q", err, tt.wantErr)
			}
			// the error must never repeat what was in the password field
			if _, secret, _ := strings.Cut(tt.line, ":"); secret != "" && strings.Contains(err.Error(), secret) {
				t.Errorf("error includes the password field: %q", err)
			}
		})
	}
}

func TestParseUsersDuplicate(t *testing.T) {
	_, err := ParseUsers(strings.NewReader("scanner:" + hash(t, "a") + "\nscanner:" + hash(t, "b") + "\n"))
	if err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Errorf("ParseUsers() error = %v, want duplicate user error", err)
	}
}

func TestLoadUsers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users")
	if err := os.WriteFile(path, []byte("scanner:"+hash(t, "scanner-password")+"\n"), 0o600); err != nil {
		t.Fatalf("could not write users file: %v", err)
	}

	users, err := LoadUsers(path)
	if err != nil {
		t.Fatalf("LoadUsers() error = %v", err)
	}
	if !users.authenticate("scanner", "scanner-password") {
		t.Error("authenticate() = false, want true")
	}

	if _, err := LoadUsers(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("LoadUsers() of a missing file: error = nil, want error")
	}
}
