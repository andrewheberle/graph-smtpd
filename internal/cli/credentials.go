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
// When no secret is set, the secret is read from the file named by the
// "secret_file" key.
func Credentials(cfg configger.Config) (tenantID, clientID, secret string, err error) {
	tenantID = cfg.String("tenantid")
	clientID = cfg.String("clientid")
	secret = cfg.String("secret")

	if secretFile := cfg.String("secret_file"); secret == "" && secretFile != "" {
		b, err := os.ReadFile(secretFile)
		if err != nil {
			return tenantID, clientID, secret, fmt.Errorf("could not read secret file %q: %w", secretFile, err)
		}
		secret = strings.TrimSpace(string(b))
	}

	return tenantID, clientID, secret, nil
}
