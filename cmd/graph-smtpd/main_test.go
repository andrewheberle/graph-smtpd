package main

import (
	"testing"
	"time"

	"github.com/andrewheberle/graph-smtpd/pkg/graphserver"
	"github.com/spf13/pflag"
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
