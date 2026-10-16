package sendmail

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/mail"
	"strings"

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

	msg, err := parseMIME(r)
	if err != nil {
		return nil, fmt.Errorf("could not parse message: %w", err)
	}

	header := msg.header

	// without an envelope sender use the address from the From header
	from := env.From
	if addrs := parseAddressList(header.Get("From")); from == "" && len(addrs) > 0 {
		from = addrs[0]
	}
	if from == "" {
		return nil, fmt.Errorf("message has no sender")
	}

	message := graphmodels.NewMessage()

	message.SetSubject(&msg.subject)
	message.SetFrom(newRecipient(from))

	to := parseAddressList(header.Get("To"))
	cc := parseAddressList(header.Get("Cc"))
	bcc := parseAddressList(header.Get("Bcc"))
	if len(env.Rcpt) > 0 {
		to, cc, bcc = envelopeRecipients(env.Rcpt, to, cc)
	}
	if len(to)+len(cc)+len(bcc) == 0 {
		return nil, fmt.Errorf("message has no recipients")
	}

	if len(to) > 0 {
		message.SetToRecipients(newRecipients(to))
	}
	if len(cc) > 0 {
		message.SetCcRecipients(newRecipients(cc))
	}
	if len(bcc) > 0 {
		message.SetBccRecipients(newRecipients(bcc))
	}

	// Graph takes a single body, so prefer HTML and fall back to text
	body := graphmodels.NewItemBody()
	if msg.htmlBody != "" {
		contentType := graphmodels.HTML_BODYTYPE
		body.SetContent(&msg.htmlBody)
		body.SetContentType(&contentType)
	} else {
		body.SetContent(&msg.textBody)
	}
	message.SetBody(body)

	if len(msg.attachments) > 0 {
		message.SetAttachments(newAttachments(msg.attachments))
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

// newAttachments converts attachments to Graph file attachments; inline
// attachments keep their content ID so an HTML body can reference them.
func newAttachments(attachments []attachment) []graphmodels.Attachmentable {
	attachmentable := make([]graphmodels.Attachmentable, 0, len(attachments))
	for _, a := range attachments {
		attachment := graphmodels.NewFileAttachment()
		attachment.SetName(&a.name)
		attachment.SetContentType(&a.contentType)
		attachment.SetContentBytes(a.data)
		if a.inline {
			attachment.SetContentId(&a.contentID)
			attachment.SetIsInline(&a.inline)
		}

		attachmentable = append(attachmentable, attachment)
	}

	return attachmentable
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

// envelopeRecipients makes the envelope authoritative for delivery: the To
// and Cc headers are kept only where the address is also an envelope
// recipient, and any envelope recipient not in those headers becomes a Bcc
// recipient. Addresses are compared case-insensitively.
func envelopeRecipients(rcpt, to, cc []string) (envTo, envCc, envBcc []string) {
	pending := make(map[string]bool, len(rcpt))
	for _, r := range rcpt {
		pending[strings.ToLower(r)] = true
	}

	// keep header recipients that are in the envelope, removing each from
	// pending so it is only delivered once
	keep := func(addrs []string) []string {
		kept := []string{}
		for _, a := range addrs {
			if key := strings.ToLower(a); pending[key] {
				kept = append(kept, a)
				delete(pending, key)
			}
		}

		return kept
	}
	envTo = keep(to)
	envCc = keep(cc)

	envBcc = []string{}
	for _, r := range rcpt {
		if key := strings.ToLower(r); pending[key] {
			envBcc = append(envBcc, r)
			delete(pending, key)
		}
	}

	return envTo, envCc, envBcc
}

func newRecipients(addresses []string) []graphmodels.Recipientable {
	recipients := make([]graphmodels.Recipientable, 0, len(addresses))
	for _, a := range addresses {
		recipients = append(recipients, newRecipient(a))
	}

	return recipients
}

func parseAddressList(addresses string) []string {
	list := []string{}

	if addresses == "" {
		return list
	}

	// Split the address list by commas, trim spaces and parse as valid email address
	for _, s := range strings.Split(addresses, ",") {
		a, err := mail.ParseAddress(strings.TrimSpace(s))
		if err != nil {
			continue
		}

		list = append(list, a.Address)
	}

	return list
}
