package graphserver

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"

	"github.com/OfimaticSRL/parsemail"
	"github.com/andrewheberle/graph-smtpd/pkg/graphclient"
	"github.com/andrewheberle/graph-smtpd/pkg/sendmail"
	"github.com/emersion/go-smtp"
	graphusers "github.com/microsoftgraph/msgraph-sdk-go/users"
	"github.com/prometheus/client_golang/prometheus"
)

type Session struct {
	from            string
	rcpt            []string
	user            *graphusers.UserItemRequestBuilder
	client          *graphclient.Client
	saveToSentItems bool
	logger          *slog.Logger
	allowedSenders  []string
	helo            string
	remote          string
	errors          []error

	sendErrors prometheus.Counter
	sendDenied prometheus.Counter
}

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	s.from = from
	s.logger = s.logger.With("from", from)

	if s.client == nil {
		err := errors.New("graph client not initialised")
		s.errors = append(s.errors, err)
		s.logger.Error("problem with graph client", "error", err)

		// increment metric
		s.sendErrors.Inc()

		return err
	}

	// check that sender is allowed
	if len(s.allowedSenders) > 0 {
		if _, found := slices.BinarySearch(s.allowedSenders, from); !found {
			err := errors.New("sender not allowed")
			s.errors = append(s.errors, err)
			s.logger.Error("problem checking allowed senders", "error", err)

			// increment metric
			s.sendDenied.Inc()

			return err
		}
	}

	// get UserItemRequestBuilder
	user := s.client.Users().ByUserId(from)
	if user == nil {
		// Some error creating user object
		err := errors.New("user not found")
		s.errors = append(s.errors, err)
		s.logger.Error("problem getting user", "error", err)

		// increment metric
		s.sendErrors.Inc()

		return err
	}

	s.user = user
	return nil
}

func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	s.rcpt = append(s.rcpt, to)

	return nil
}

func (s *Session) Data(r io.Reader) error {
	// parse incoming message
	msg, err := parsemail.Parse(r)
	if err != nil {
		s.errors = append(s.errors, err)
		s.logger.Error("problem in parsing mail data", "error", err)
		// increment metric
		s.sendErrors.Inc()

		return err
	}

	// grab headers and content
	header := msg.Header
	subject := header.Get("Subject")
	from := s.from
	to := header.Get("To")
	cc := header.Get("Cc")
	bcc := header.Get("Bcc")
	s.logger = s.logger.With(
		"subject", subject,
		"to", to,
		"cc", cc,
		"bcc", bcc,
	)

	// message options
	opts := []sendmail.MessageOption{
		sendmail.WithCc(cc),
		sendmail.WithBcc(bcc),
		sendmail.WithAttachments(msg.Attachments),
		sendmail.WithSaveToSentItems(s.saveToSentItems),
	}

	// handle HTML or text bodies
	if msg.TextBody == "" {
		opts = append(opts, sendmail.WithBody(msg.HTMLBody), sendmail.WithHTMLContent())
	} else if msg.HTMLBody == "" {
		opts = append(opts, sendmail.WithBody(msg.TextBody))
	}

	// create message to send
	message := sendmail.NewMessage(from, to, subject, opts...)

	// send it
	if err := message.Send(context.Background(), s.user); err != nil {
		s.errors = append(s.errors, err)
		s.logger.Error("problem sending message", "error", err)

		// increment metric
		s.sendErrors.Inc()

		return err
	}

	s.logger.Debug("message sent", "rcpt", s.rcpt)

	return nil
}

func (s *Session) Reset() {
	if err := errors.Join(s.errors...); err != nil {
		s.logger.Error("session ended", "errors", s.errors, "rcpt", s.rcpt)
	} else {
		s.logger.Info("session ended", "rcpt", s.rcpt)
	}
}

func (s *Session) Logout() error {
	return nil
}
