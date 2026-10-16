package main

import (
	"testing"

	"github.com/andrewheberle/configger"
	"github.com/spf13/pflag"
)

func TestCredentials(t *testing.T) {
	f := pflag.NewFlagSet("config", pflag.ContinueOnError)
	f.String("clientid", "", "")
	f.String("tenantid", "", "")
	f.String("secret", "", "")
	if err := f.Parse([]string{"--tenantid", "tenant", "--clientid", "client", "--secret", "secret"}); err != nil {
		t.Fatalf("could not parse flags: %v", err)
	}

	k, err := configger.LoadConfig(f, configger.WithoutConfigurationFile())
	if err != nil {
		t.Fatalf("could not load config: %v", err)
	}

	tenantID, clientID, secret := credentials(k)
	if tenantID != "tenant" || clientID != "client" || secret != "secret" {
		t.Errorf("credentials() = %q, %q, %q, want %q, %q, %q", tenantID, clientID, secret, "tenant", "client", "secret")
	}
}
