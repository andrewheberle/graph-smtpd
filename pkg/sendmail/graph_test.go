package sendmail

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestGraphMailerSendsInternetMessageID(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// the Graph SDK compresses request bodies
		reader := io.Reader(r.Body)
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				t.Errorf("could not decompress request: %v", err)
				return
			}
			reader = gz
		}
		body, _ = io.ReadAll(reader)
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)

	adapter, err := graph.NewGraphRequestAdapter(&authentication.AnonymousAuthenticationProvider{})
	if err != nil {
		t.Fatalf("could not create request adapter: %v", err)
	}
	adapter.SetBaseUrl(srv.URL + "/v1.0")
	mailer := newGraphMailer(graph.NewGraphServiceClient(adapter))

	m, err := FromMIME(strings.NewReader("From: alice@example.com\nTo: bob@example.com\nMessage-ID: <abc123@example.com>\n\nHello.\n"), Envelope{})
	if err != nil {
		t.Fatalf("FromMIME() error = %v", err)
	}
	if err := mailer.Send(context.Background(), m); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	var request struct {
		Message struct {
			InternetMessageID string `json:"internetMessageId"`
		} `json:"message"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatalf("request body is not JSON: %v: %s", err, body)
	}
	if request.Message.InternetMessageID != "<abc123@example.com>" {
		t.Errorf("internetMessageId = %q, want %q; body: %s", request.Message.InternetMessageID, "<abc123@example.com>", body)
	}
}
