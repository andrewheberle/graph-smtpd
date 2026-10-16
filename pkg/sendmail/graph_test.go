package sendmail

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/microsoft/kiota-abstractions-go/authentication"
	graph "github.com/microsoftgraph/msgraph-sdk-go"
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

// graphServer runs a fake Graph endpoint that answers every request with
// status and body, and returns a GraphMailer that talks to it.
func graphServer(t *testing.T, status int, body string) *GraphMailer {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)

	adapter, err := graph.NewGraphRequestAdapter(&authentication.AnonymousAuthenticationProvider{})
	if err != nil {
		t.Fatalf("could not create request adapter: %v", err)
	}
	adapter.SetBaseUrl(srv.URL + "/v1.0")

	return newGraphMailer(graph.NewGraphServiceClient(adapter))
}

func graphError(code string) string {
	return `{"error":{"code":"` + code + `","message":"test error"}}`
}

func TestGraphMailerSend(t *testing.T) {
	tests := []struct {
		name         string
		status       int
		body         string
		wantErr      bool
		wantRejected bool
		wantCode     string
	}{
		{"accepted", http.StatusAccepted, "", false, false, ""},
		{"access denied is rejected", http.StatusForbidden, graphError("ErrorAccessDenied"), true, true, "ErrorAccessDenied"},
		{"unknown mailbox is rejected", http.StatusNotFound, graphError("ErrorInvalidUser"), true, true, "ErrorInvalidUser"},
		{"too large is rejected", http.StatusRequestEntityTooLarge, graphError("ErrorMessageSizeExceeded"), true, true, "ErrorMessageSizeExceeded"},
		{"bad request is rejected", http.StatusBadRequest, graphError("ErrorInvalidRecipients"), true, true, "ErrorInvalidRecipients"},
		{"unauthorised is temporary", http.StatusUnauthorized, graphError("InvalidAuthenticationToken"), true, false, ""},
		{"server error is temporary", http.StatusInternalServerError, graphError("ErrorInternalServerError"), true, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mailer := graphServer(t, tt.status, tt.body)
			m := fromFixture(t, "plain.eml", Envelope{From: "alice@example.com"})

			err := mailer.Send(context.Background(), m)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Send() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got := errors.Is(err, ErrRejected); got != tt.wantRejected {
				t.Errorf("errors.Is(err, ErrRejected) = %v, want %v (err = %v)", got, tt.wantRejected, err)
			}

			var rejected *RejectedError
			if errors.As(err, &rejected) {
				if rejected.StatusCode != tt.status {
					t.Errorf("StatusCode = %d, want %d", rejected.StatusCode, tt.status)
				}
				if rejected.Code != tt.wantCode {
					t.Errorf("Code = %q, want %q", rejected.Code, tt.wantCode)
				}
			}
		})
	}
}

func TestGraphMailerSendNetworkErrorIsTemporary(t *testing.T) {
	mailer := graphServer(t, http.StatusAccepted, "")
	m := fromFixture(t, "plain.eml", Envelope{From: "alice@example.com"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := mailer.Send(ctx, m)
	if err == nil {
		t.Fatal("Send() error = nil, want error")
	}
	if errors.Is(err, ErrRejected) {
		t.Errorf("errors.Is(err, ErrRejected) = true, want false (err = %v)", err)
	}
}
