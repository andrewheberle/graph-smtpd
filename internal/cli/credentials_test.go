package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/andrewheberle/configger"
	"github.com/spf13/pflag"
)

// loadConfig loads configuration the way the commands do, from args and from
// environment variables with prefix.
func loadConfig(t *testing.T, prefix string, args ...string) configger.Config {
	t.Helper()

	f := pflag.NewFlagSet("test", pflag.ContinueOnError)
	f.String("config", "", "")
	f.String("clientid", "", "")
	f.String("tenantid", "", "")
	f.String("secret", "", "")
	f.String("secretfile", "", "")
	if err := f.Parse(args); err != nil {
		t.Fatalf("could not parse flags: %v", err)
	}

	cfg, err := configger.LoadConfig(f, configger.WithEnvPrefix(prefix))
	if err != nil {
		t.Fatalf("could not load config: %v", err)
	}

	return cfg
}

// writeFile writes content to a new file in a temporary directory and
// returns its path.
func writeFile(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("could not write %s: %v", name, err)
	}

	return path
}

func TestCredentialsFromFlags(t *testing.T) {
	cfg := loadConfig(t, "TEST", "--tenantid", "tenant", "--clientid", "client", "--secret", "secret")

	tenantID, clientID, secret, err := Credentials(cfg)
	if err != nil {
		t.Fatalf("Credentials() error = %v", err)
	}
	if tenantID != "tenant" || clientID != "client" || secret != "secret" {
		t.Errorf("Credentials() = %q, %q, %q, want %q, %q, %q", tenantID, clientID, secret, "tenant", "client", "secret")
	}
}

func TestCredentialsSecretFileFromConfigFile(t *testing.T) {
	secretFile := writeFile(t, "secret", "file-secret\n")
	config := writeFile(t, "config.yaml", "secretfile: "+secretFile+"\n")
	cfg := loadConfig(t, "TEST", "--config", config)

	_, _, secret, err := Credentials(cfg)
	if err != nil {
		t.Fatalf("Credentials() error = %v", err)
	}
	if secret != "file-secret" {
		t.Errorf("secret = %q, want %q", secret, "file-secret")
	}
}

func TestCredentialsSecretFileWinsOverSecret(t *testing.T) {
	secretFile := writeFile(t, "secret", "file-secret")
	config := writeFile(t, "config.yaml", "secretfile: "+secretFile+"\n")
	cfg := loadConfig(t, "TEST", "--config", config, "--secret", "flag-secret")

	_, _, secret, err := Credentials(cfg)
	if err != nil {
		t.Fatalf("Credentials() error = %v", err)
	}
	if secret != "file-secret" {
		t.Errorf("secret = %q, want %q", secret, "file-secret")
	}
}

func TestCredentialsUnreadableSecretFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	config := writeFile(t, "config.yaml", "secretfile: "+missing+"\n")
	cfg := loadConfig(t, "TEST", "--config", config)

	if _, _, _, err := Credentials(cfg); err == nil {
		t.Errorf("Credentials() error = nil, want error")
	}
}

func TestCredentialsSecretFileFromEnv(t *testing.T) {
	secretFile := writeFile(t, "secret", "file-secret\n")

	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"secret file only", map[string]string{"TEST_SECRETFILE": secretFile}, "file-secret"},
		{"secret file wins over secret", map[string]string{"TEST_SECRETFILE": secretFile, "TEST_SECRET": "env-secret"}, "file-secret"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			cfg := loadConfig(t, "TEST")

			_, _, secret, err := Credentials(cfg)
			if err != nil {
				t.Fatalf("Credentials() error = %v", err)
			}
			if secret != tt.want {
				t.Errorf("secret = %q, want %q, %q", secret, tt.want, cfg.String("secret"))
			}
		})
	}
}
