package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andrewheberle/graph-smtpd/internal/cli"
	"github.com/spf13/pflag"
)

func flags(t *testing.T, args ...string) *pflag.FlagSet {
	t.Helper()

	f := pflag.NewFlagSet("config", pflag.ContinueOnError)
	f.String("config", "", "")
	f.String("clientid", "", "")
	f.String("tenantid", "", "")
	f.String("secret", "", "")
	if err := f.Parse(args); err != nil {
		t.Fatalf("could not parse flags: %v", err)
	}

	return f
}

func TestLoadConfigCredentialsFromFlags(t *testing.T) {
	cfg, err := loadConfig(flags(t, "--tenantid", "tenant", "--clientid", "client", "--secret", "secret"))
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}

	tenantID, clientID, secret, err := cli.Credentials(cfg)
	if err != nil {
		t.Fatalf("Credentials() error = %v", err)
	}
	if tenantID != "tenant" || clientID != "client" || secret != "secret" {
		t.Errorf("Credentials() = %q, %q, %q, want %q, %q, %q", tenantID, clientID, secret, "tenant", "client", "secret")
	}
}

func TestLoadConfigSecretFileFromEnv(t *testing.T) {
	secretFile := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secretFile, []byte("file-secret\n"), 0o600); err != nil {
		t.Fatalf("could not write secret file: %v", err)
	}
	t.Setenv("SENDMAIL_SECRET_FILE", secretFile)

	cfg, err := loadConfig(flags(t))
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}

	_, _, secret, err := cli.Credentials(cfg)
	if err != nil {
		t.Fatalf("Credentials() error = %v", err)
	}
	if secret != "file-secret" {
		t.Errorf("secret = %q, want %q", secret, "file-secret")
	}
}

// runMain runs main in a subprocess with args, returning its stderr and
// exit error.
func runMain(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=^TestMainProcess$")
	cmd.Env = append(os.Environ(), "GRAPH_SENDMAIL_TEST_MAIN=1", "GRAPH_SENDMAIL_TEST_ARGS="+strings.Join(args, "\x1f"))
	cmd.Stdin = strings.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()

	return stderr.String(), err
}

// TestMainProcess is not a real test: runMain uses it to run main in a
// subprocess.
func TestMainProcess(t *testing.T) {
	if os.Getenv("GRAPH_SENDMAIL_TEST_MAIN") != "1" {
		t.Skip("only run as a subprocess by runMain")
	}

	os.Args = append([]string{"graph-sendmail"}, strings.Split(os.Getenv("GRAPH_SENDMAIL_TEST_ARGS"), "\x1f")...)
	main()
}

func TestQuietSilencesErrors(t *testing.T) {
	tests := []struct {
		name  string
		stdin string
		args  []string
	}{
		{"missing credentials", "", []string{"--quiet"}},
		{"unreadable message", "", []string{"--quiet", "--tenantid", "tenant", "--clientid", "client", "--secret", "secret"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stderr, err := runMain(t, tt.stdin, tt.args...)

			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
				t.Fatalf("exit = %v, want exit status 1", err)
			}
			if stderr != "" {
				t.Errorf("stderr = %q, want nothing with --quiet", stderr)
			}
		})
	}
}
