package sendmail

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/mail"
	"strings"

	"github.com/OfimaticSRL/parsemail"
	graphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
	graphusers "github.com/microsoftgraph/msgraph-sdk-go/users"
)

// Envelope is the SMTP envelope for a message.
//
// A zero Envelope means there is no envelope, and the message headers are
// used in its place.
type Envelope struct {
	From string
	Rcpt []string
}

type Message struct {
	from        string
	message     graphmodels.Messageable
	requestBody *graphusers.ItemSendMailPostRequestBody
}

// FromMIME parses the MIME message read from r and builds a Message ready
// to send via Microsoft Graph.
func FromMIME(r io.Reader, env Envelope, opts ...Option) (*Message, error) {
	o := new(options)
	for _, opt := range opts {
		opt(o)
	}

	msg, err := parsemail.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("could not parse message: %w", err)
	}

	header := msg.Header

	// without an envelope sender use the address from the From header
	from := env.From
	if from == "" && len(msg.From) > 0 {
		from = msg.From[0].Address
	}
	if from == "" {
		return nil, fmt.Errorf("message has no sender")
	}

	message := graphmodels.NewMessage()

	subject := header.Get("Subject")
	message.SetSubject(&subject)
	message.SetFrom(newRecipient(from))

	if addrs := parseAddressList(header.Get("To")); len(addrs) > 0 {
		message.SetToRecipients(addrs)
	}
	if addrs := parseAddressList(header.Get("Cc")); len(addrs) > 0 {
		message.SetCcRecipients(addrs)
	}
	if addrs := parseAddressList(header.Get("Bcc")); len(addrs) > 0 {
		message.SetBccRecipients(addrs)
	}

	// Graph takes a single body, so prefer HTML and fall back to text
	body := graphmodels.NewItemBody()
	if msg.HTMLBody != "" {
		contentType := graphmodels.HTML_BODYTYPE
		body.SetContent(&msg.HTMLBody)
		body.SetContentType(&contentType)
	} else {
		body.SetContent(&msg.TextBody)
	}
	message.SetBody(body)

	attachments, err := newAttachments(msg.Attachments)
	if err != nil {
		return nil, err
	}
	if len(attachments) > 0 {
		message.SetAttachments(attachments)
	}

	requestBody := graphusers.NewItemSendMailPostRequestBody()
	requestBody.SetSaveToSentItems(&o.saveToSentItems)

	return &Message{
		from:        from,
		message:     message,
		requestBody: requestBody,
	}, nil
}

// From returns the address of the mailbox the Message is sent from.
func (m *Message) From() string {
	return m.from
}

func (m *Message) Send(ctx context.Context, user *graphusers.UserItemRequestBuilder) error {
	m.requestBody.SetMessage(m.message)

	return user.SendMail().Post(ctx, m.requestBody, nil)
}

// LogValue implements slog.LogValuer.
func (m *Message) LogValue() slog.Value {
	bodyType := "text/plain"
	bodySize := 0
	if body := m.message.GetBody(); body != nil {
		if t := body.GetContentType(); t != nil && *t == graphmodels.HTML_BODYTYPE {
			bodyType = "text/html"
		}
		if c := body.GetContent(); c != nil {
			bodySize = len(*c)
		}
	}

	subject := ""
	if s := m.message.GetSubject(); s != nil {
		subject = *s
	}

	return slog.GroupValue(
		slog.String("from", m.from),
		slog.String("subject", subject),
		slog.Any("to", addresses(m.message.GetToRecipients())),
		slog.Any("cc", addresses(m.message.GetCcRecipients())),
		slog.Any("bcc", addresses(m.message.GetBccRecipients())),
		slog.String("type", bodyType),
		slog.Int("size", bodySize),
		slog.Int("attachments", len(m.message.GetAttachments())),
	)
}

func newRecipient(address string) graphmodels.Recipientable {
	recipient := graphmodels.NewRecipient()
	emailAddress := graphmodels.NewEmailAddress()
	emailAddress.SetAddress(&address)
	recipient.SetEmailAddress(emailAddress)

	return recipient
}

func newAttachments(attachments []parsemail.Attachment) ([]graphmodels.Attachmentable, error) {
	attachmentable := []graphmodels.Attachmentable{}
	for _, a := range attachments {
		data, err := io.ReadAll(a.Data)
		if err != nil {
			return nil, fmt.Errorf("could not read attachment %q: %w", a.Filename, err)
		}
		attachment := graphmodels.NewFileAttachment()
		attachment.SetName(&a.Filename)
		attachment.SetContentType(&a.ContentType)
		attachment.SetContentBytes(data)

		attachmentable = append(attachmentable, attachment)
	}

	return attachmentable, nil
}

// addresses returns the email addresses of recipients
func addresses(recipients []graphmodels.Recipientable) []string {
	list := make([]string, 0, len(recipients))
	for _, r := range recipients {
		if e := r.GetEmailAddress(); e != nil && e.GetAddress() != nil {
			list = append(list, *e.GetAddress())
		}
	}

	return list
}

func parseAddressList(addresses string) []graphmodels.Recipientable {
	recipientList := []graphmodels.Recipientable{}

	if addresses == "" {
		return recipientList
	}

	// Split the address list by commas, trim spaces and parse as valid email address
	list := strings.Split(addresses, ",")
	for i := range list {
		a, err := mail.ParseAddress(strings.TrimSpace(list[i]))
		if err != nil {
			continue
		}

		recipientList = append(recipientList, newRecipient(a.Address))
	}

	return recipientList
}
