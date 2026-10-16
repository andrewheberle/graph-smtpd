package sendmail

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/iotest"

	"github.com/OfimaticSRL/parsemail"

	graphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
)

func fromFixture(t *testing.T, name string, env Envelope, opts ...Option) *Message {
	t.Helper()

	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("could not open fixture: %v", err)
	}
	defer func() { _ = f.Close() }()

	m, err := FromMIME(f, env, opts...)
	if err != nil {
		t.Fatalf("FromMIME() error = %v", err)
	}

	return m
}

func body(m *Message) (content string, html bool) {
	b := m.message.GetBody()
	if c := b.GetContent(); c != nil {
		content = *c
	}
	if t := b.GetContentType(); t != nil && *t == graphmodels.HTML_BODYTYPE {
		html = true
	}

	return content, html
}

func TestFromMIME(t *testing.T) {
	tests := []struct {
		name        string
		fixture     string
		env         Envelope
		wantFrom    string
		wantSubject string
		wantTo      []string
		wantCc      []string
		wantBcc     []string
		wantBody    string
		wantHTML    bool
	}{
		{
			name:        "plain text with envelope sender",
			fixture:     "plain.eml",
			env:         Envelope{From: "alice@example.com"},
			wantFrom:    "alice@example.com",
			wantSubject: "Plain text",
			wantTo:      []string{"bob@example.com", "carol@example.com"},
			wantCc:      []string{"dave@example.com"},
			wantBcc:     []string{"erin@example.com"},
			wantBody:    "Hello Bob.",
		},
		{
			name:        "html only",
			fixture:     "html.eml",
			env:         Envelope{From: "alice@example.com"},
			wantFrom:    "alice@example.com",
			wantSubject: "HTML only",
			wantTo:      []string{"bob@example.com"},
			wantCc:      []string{},
			wantBcc:     []string{},
			wantBody:    "<p>Hello Bob.</p>",
			wantHTML:    true,
		},
		{
			name:        "text with attachment",
			fixture:     "attachment.eml",
			env:         Envelope{From: "alice@example.com"},
			wantFrom:    "alice@example.com",
			wantSubject: "With attachment",
			wantTo:      []string{"bob@example.com"},
			wantCc:      []string{},
			wantBcc:     []string{},
			wantBody:    "See attached.",
		},
		{
			name:        "text and html prefers html",
			fixture:     "alternative.eml",
			env:         Envelope{From: "alice@example.com"},
			wantFrom:    "alice@example.com",
			wantSubject: "Text and HTML",
			wantTo:      []string{"bob@example.com"},
			wantCc:      []string{},
			wantBcc:     []string{},
			wantBody:    "<p>Hello Bob.</p>",
			wantHTML:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := fromFixture(t, tt.fixture, tt.env)

			if got := m.From(); got != tt.wantFrom {
				t.Errorf("From() = %q, want %q", got, tt.wantFrom)
			}
			if got := addresses([]graphmodels.Recipientable{m.message.GetFrom()}); !slices.Equal(got, []string{tt.wantFrom}) {
				t.Errorf("message from = %v, want %v", got, tt.wantFrom)
			}
			if got := *m.message.GetSubject(); got != tt.wantSubject {
				t.Errorf("subject = %q, want %q", got, tt.wantSubject)
			}
			if got := addresses(m.message.GetToRecipients()); !slices.Equal(got, tt.wantTo) {
				t.Errorf("to = %v, want %v", got, tt.wantTo)
			}
			if got := addresses(m.message.GetCcRecipients()); !slices.Equal(got, tt.wantCc) {
				t.Errorf("cc = %v, want %v", got, tt.wantCc)
			}
			if got := addresses(m.message.GetBccRecipients()); !slices.Equal(got, tt.wantBcc) {
				t.Errorf("bcc = %v, want %v", got, tt.wantBcc)
			}
			content, html := body(m)
			if content != tt.wantBody {
				t.Errorf("body = %q, want %q", content, tt.wantBody)
			}
			if html != tt.wantHTML {
				t.Errorf("html = %v, want %v", html, tt.wantHTML)
			}
		})
	}
}

func TestFromMIMEAttachments(t *testing.T) {
	m := fromFixture(t, "attachment.eml", Envelope{From: "alice@example.com"})

	attachments := m.message.GetAttachments()
	if len(attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(attachments))
	}

	a, ok := attachments[0].(graphmodels.FileAttachmentable)
	if !ok {
		t.Fatalf("attachment is %T, want FileAttachmentable", attachments[0])
	}
	if got := *a.GetName(); got != "notes.txt" {
		t.Errorf("name = %q, want %q", got, "notes.txt")
	}
	if got := *a.GetContentType(); got != "text/plain" {
		t.Errorf("content type = %q, want %q", got, "text/plain")
	}
	if got := string(a.GetContentBytes()); got != "hello world" {
		t.Errorf("content = %q, want %q", got, "hello world")
	}
}

func TestFromMIMESaveToSentItems(t *testing.T) {
	for _, save := range []bool{false, true} {
		m := fromFixture(t, "plain.eml", Envelope{From: "alice@example.com"}, WithSaveToSentItems(save))
		if got := *m.requestBody.GetSaveToSentItems(); got != save {
			t.Errorf("saveToSentItems = %v, want %v", got, save)
		}
	}
}

func TestFromMIMEHeaderSender(t *testing.T) {
	m := fromFixture(t, "plain.eml", Envelope{})

	if got, want := m.From(), "alice@example.com"; got != want {
		t.Errorf("From() = %q, want %q", got, want)
	}
}

func TestFromMIMENoSender(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "nosender.eml"))
	if err != nil {
		t.Fatalf("could not open fixture: %v", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := FromMIME(f, Envelope{}); err == nil {
		t.Errorf("FromMIME() error = nil, want error")
	}
}

func TestNewAttachmentsReadError(t *testing.T) {
	attachments := []parsemail.Attachment{
		{Filename: "bad.txt", ContentType: "text/plain", Data: iotest.ErrReader(errors.New("read failed"))},
	}

	if _, err := newAttachments(attachments); err == nil {
		t.Errorf("newAttachments() error = nil, want error")
	}
}

func TestFromMIMEInlineAttachments(t *testing.T) {
	m := fromFixture(t, "inline.eml", Envelope{From: "alice@example.com"})

	attachments := m.message.GetAttachments()
	if len(attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(attachments))
	}

	a, ok := attachments[0].(graphmodels.FileAttachmentable)
	if !ok {
		t.Fatalf("attachment is %T, want FileAttachmentable", attachments[0])
	}
	if got := a.GetContentId(); got == nil || *got != "logo@example.com" {
		t.Errorf("content id = %v, want %q", got, "logo@example.com")
	}
	if got := a.GetIsInline(); got == nil || !*got {
		t.Errorf("inline = %v, want true", got)
	}
	if got := *a.GetContentType(); got != "image/png" {
		t.Errorf("content type = %q, want %q", got, "image/png")
	}
	if got := string(a.GetContentBytes()); got != "\x89PNG\r\n\x1a\n" {
		t.Errorf("content = %q, want PNG signature", got)
	}
}

func TestFromMIMEEnvelopeRecipients(t *testing.T) {
	// plain.eml headers: To bob, carol; Cc dave; Bcc erin
	env := Envelope{
		From: "alice@example.com",
		Rcpt: []string{"BOB@example.com", "dave@example.com", "frank@example.com", "Frank@example.com"},
	}
	m := fromFixture(t, "plain.eml", env)

	if got, want := addresses(m.message.GetToRecipients()), []string{"bob@example.com"}; !slices.Equal(got, want) {
		t.Errorf("to = %v, want %v", got, want)
	}
	if got, want := addresses(m.message.GetCcRecipients()), []string{"dave@example.com"}; !slices.Equal(got, want) {
		t.Errorf("cc = %v, want %v", got, want)
	}
	if got, want := addresses(m.message.GetBccRecipients()), []string{"frank@example.com"}; !slices.Equal(got, want) {
		t.Errorf("bcc = %v, want %v", got, want)
	}
}
