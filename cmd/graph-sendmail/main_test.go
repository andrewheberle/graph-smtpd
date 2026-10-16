package main

import (
	"os"
	"path/filepath"
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
