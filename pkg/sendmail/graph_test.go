package sendmail

import (
	"testing"
)

func TestNewGraphMailer(t *testing.T) {
	tests := []struct {
		name     string
		tenantid string
		clientid string
		secret   string
		wantErr  bool
	}{
		{"missing all", "", "", "", true},
		{"missing tenantid", "", "clientid", "secret", true},
		{"missing clientid", "tenantid", "", "secret", true},
		{"missing secret", "tenantid", "clientid", "", true},
		{"have all", "tenantid", "clientid", "secret", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewGraphMailer(tt.tenantid, tt.clientid, tt.secret)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewGraphMailer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
