package graphserver

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"

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
	msgLogger       *slog.Logger
	allowedSenders  []string
	helo            string
	remote          string
	errors          []error

	sendErrors prometheus.Counter
	sendDenied prometheus.Counter
}

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	s.from = from
	s.msgLogger = s.log().With("from", from)

	if s.client == nil {
		err := errors.New("graph client not initialised")
		s.errors = append(s.errors, err)
		s.log().Error("problem with graph client", "error", err)

		// increment metric
		s.sendErrors.Inc()

		return err
	}

	// check that sender is allowed
	if len(s.allowedSenders) > 0 {
		if _, found := slices.BinarySearch(s.allowedSenders, from); !found {
			err := errors.New("sender not allowed")
			s.errors = append(s.errors, err)
			s.log().Error("problem checking allowed senders", "error", err)

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
		s.log().Error("problem getting user", "error", err)

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
	// build message from incoming data
	env := sendmail.Envelope{From: s.from, Rcpt: s.rcpt}
	message, err := sendmail.FromMIME(r, env, sendmail.WithSaveToSentItems(s.saveToSentItems))
	if err != nil {
		s.errors = append(s.errors, err)
		s.log().Error("problem in parsing mail data", "error", err)
		// increment metric
		s.sendErrors.Inc()

		return err
	}

	s.msgLogger = s.log().With("message", message)

	// send it
	if err := message.Send(context.Background(), s.user); err != nil {
		s.errors = append(s.errors, err)
		s.log().Error("problem sending message", "error", err)

		// increment metric
		s.sendErrors.Inc()

		return err
	}

	s.log().Debug("message sent", "rcpt", s.rcpt)

	return nil
}

// Reset discards the current message, so the next message in the session
// starts with a clean envelope.
func (s *Session) Reset() {
	if err := errors.Join(s.errors...); err != nil {
		s.log().Error("session ended", "errors", s.errors, "rcpt", s.rcpt)
	} else {
		s.log().Info("session ended", "rcpt", s.rcpt)
	}

	s.from = ""
	s.rcpt = nil
	s.user = nil
	s.errors = nil
	s.msgLogger = nil
}

// log returns the logger for the current message, falling back to the
// session logger before a message has started.
func (s *Session) log() *slog.Logger {
	if s.msgLogger != nil {
		return s.msgLogger
	}

	return s.logger
}

func (s *Session) Logout() error {
	return nil
}
