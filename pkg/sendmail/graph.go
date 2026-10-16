package sendmail

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	graph "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
)

// ErrRejected matches a RejectedError with errors.Is.
var ErrRejected = errors.New("message rejected by Microsoft Graph")

// RejectedError is returned by GraphMailer.Send when Graph refuses a message
// in a way that retrying cannot fix, such as the app not being allowed to
// send as the mailbox (403) or the mailbox not existing (404). Any other
// failure is temporary and returned as is.
type RejectedError struct {
	// StatusCode is the HTTP status Graph returned.
	StatusCode int
	// Code is Graph's error code, such as "ErrorAccessDenied", if it gave one.
	Code string

	err error
}

func (e *RejectedError) Error() string {
	return fmt.Sprintf("%s (%d %s): %v", ErrRejected, e.StatusCode, e.Code, e.err)
}

func (e *RejectedError) Unwrap() error { return e.err }

func (e *RejectedError) Is(target error) bool { return target == ErrRejected }

// classify returns err as a *RejectedError when Graph's status shows the
// message can never be delivered, and err unchanged otherwise.
func classify(err error) error {
	var apiErr interface{ GetStatusCode() int }
	if !errors.As(err, &apiErr) {
		// not a Graph response, such as a network or authentication error
		return err
	}

	status := apiErr.GetStatusCode()
	switch {
	case status == http.StatusUnauthorized, status == http.StatusRequestTimeout, status == http.StatusTooManyRequests:
		// credentials can be fixed and requests retried
		return err
	case status < 400 || status >= 500:
		return err
	}

	rejected := &RejectedError{StatusCode: status, err: err}
	var odataErr *odataerrors.ODataError
	if errors.As(err, &odataErr) {
		if main := odataErr.GetErrorEscaped(); main != nil && main.GetCode() != nil {
			rejected.Code = *main.GetCode()
		}
	}

	return rejected
}

// GraphMailer sends a Message via Microsoft Graph, as the mailbox the
// Message is from.
type GraphMailer struct {
	client *graph.GraphServiceClient
}

// NewGraphMailer creates a GraphMailer that authenticates as an Entra ID app
// registration using a client secret.
func NewGraphMailer(tenantID, clientID, secret string) (*GraphMailer, error) {
	if tenantID == "" || clientID == "" || secret == "" {
		return nil, fmt.Errorf("tenantid, clientid and secret must not be blank")
	}

	cred, err := azidentity.NewClientSecretCredential(tenantID, clientID, secret, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create cred from secret: %w", err)
	}

	client, err := graph.NewGraphServiceClientWithCredentials(cred, []string{"https://graph.microsoft.com/.default"})
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	return newGraphMailer(client), nil
}

func newGraphMailer(client *graph.GraphServiceClient) *GraphMailer {
	return &GraphMailer{client: client}
}

// Send sends m via the sendMail endpoint of the mailbox m is from. A failure
// that retrying cannot fix is returned as a *RejectedError.
func (g *GraphMailer) Send(ctx context.Context, m *Message) error {
	m.requestBody.SetMessage(m.message)

	if err := g.client.Users().ByUserId(m.From()).SendMail().Post(ctx, m.requestBody, nil); err != nil {
		return classify(err)
	}

	return nil
}
