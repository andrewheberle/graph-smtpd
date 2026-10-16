// Package cli holds configuration helpers shared by the graph-smtpd and
// graph-sendmail commands.
package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/andrewheberle/configger"
)

// Credentials returns the Entra ID tenant ID, client ID and client secret
// from cfg.
//
// When no secret is set, it is read from a file instead: either one named by
// a <PREFIX>_SECRET_FILE environment variable or by the "secret_file" key in
// a configuration file.
//
// The environment variable loads as the "secret.file" key, which turns the
// "secret" key into a map. When <PREFIX>_SECRET is also set it replaces that
// map, so the "secret.file" key is only present when no secret was given.
func Credentials(cfg configger.Config) (tenantID, clientID, secret string, err error) {
	tenantID = cfg.String("tenantid")
	clientID = cfg.String("clientid")

	secretFile := cfg.String("secret.file")
	if secretFile == "" {
		secret = cfg.String("secret")
		if secret == "" {
			secretFile = cfg.String("secret_file")
		}
	}

	if secretFile != "" {
		b, err := os.ReadFile(secretFile)
		if err != nil {
			return tenantID, clientID, "", fmt.Errorf("could not read secret file %q: %w", secretFile, err)
		}
		secret = strings.TrimSpace(string(b))
	}

	return tenantID, clientID, secret, nil
}
