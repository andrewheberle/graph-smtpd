package sendmail

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	graph "github.com/microsoftgraph/msgraph-sdk-go"
)

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

	return &GraphMailer{client: client}, nil
}

// Send sends m via the sendMail endpoint of the mailbox m is from.
func (g *GraphMailer) Send(ctx context.Context, m *Message) error {
	m.requestBody.SetMessage(m.message)

	return g.client.Users().ByUserId(m.From()).SendMail().Post(ctx, m.requestBody, nil)
}
