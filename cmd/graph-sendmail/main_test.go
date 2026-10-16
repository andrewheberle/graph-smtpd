package main

import (
	"testing"

	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/v2"
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

	k := koanf.New(".")
	if err := k.Load(posflag.Provider(f, ".", k), nil); err != nil {
		t.Fatalf("could not load flags: %v", err)
	}

	tenantID, clientID, secret := credentials(k)
	if tenantID != "tenant" || clientID != "client" || secret != "secret" {
		t.Errorf("credentials() = %q, %q, %q, want %q, %q, %q", tenantID, clientID, secret, "tenant", "client", "secret")
	}
}
