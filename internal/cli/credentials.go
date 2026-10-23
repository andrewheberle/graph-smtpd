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
// The client secret is set from the contents of a file named in the
// <PREFIX>_SECRETFILE environment variable or by the "secretfile" key in
// a configuration file.
//
// The client secret can also be provided directly in the <PREFIX>_SECRET
// environment variable, the "secret" key in the configuration file or the
// "secret" command line flag.
//
// <PREFIX>_SECRETFILE (or "secretfile") takes precedence over <PREFIX>_SECRET
// (or "secret")
func Credentials(cfg configger.Config) (tenantID, clientID, secret string, err error) {
	tenantID = cfg.String("tenantid")
	clientID = cfg.String("clientid")

	secretFile := cfg.String("secretfile")
	if secretFile != "" {
		b, err := os.ReadFile(secretFile)
		if err != nil {
			return tenantID, clientID, "", fmt.Errorf("could not read secret file %q: %w", secretFile, err)
		}
		secret = strings.TrimSpace(string(b))
	} else {
		secret = cfg.String("secret")
	}

	return tenantID, clientID, secret, nil
}
