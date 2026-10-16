package graphserver

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/andrewheberle/graph-smtpd/pkg/sendmail"
	"github.com/emersion/go-smtp"
	"github.com/prometheus/client_golang/prometheus"
)

type Session struct {
	from            string
	rcpt            []string
	mailer          Mailer
	saveToSentItems bool
	sendTimeout     time.Duration
	logger          *slog.Logger
	msgLogger       *slog.Logger
	policy          *relayPolicy
	sourceDenied    bool
	helo            string
	remote          string
	errors          []error

	emailTotal prometheus.Counter
	sendErrors prometheus.Counter
	sendDenied prometheus.Counter
}

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	s.from = from
	s.msgLogger = s.log().With("from", from)

	// check that the client's address may relay
	if s.sourceDenied {
		err := errSourceDenied
		s.errors = append(s.errors, err)
		s.log().Error("problem checking allowed sources", "error", err, "remote", s.remote)

		// increment metric
		s.sendDenied.Inc()

		return err
	}

	// check that sender is allowed
	if !s.policy.allowSender(from) {
		err := errSenderDenied
		s.errors = append(s.errors, err)
		s.log().Error("problem checking allowed senders", "error", err)

		// increment metric
		s.sendDenied.Inc()

		return err
	}

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

		// the message itself is unusable, so retrying will not help
		return &smtp.SMTPError{
			Code:         554,
			EnhancedCode: smtp.EnhancedCode{5, 6, 0},
			Message:      "message could not be processed: " + err.Error(),
		}
	}

	s.msgLogger = s.log().With("message", message)

	// send it, within the send timeout if one is set
	ctx := context.Background()
	if s.sendTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.sendTimeout)
		defer cancel()
	}

	if err := s.mailer.Send(ctx, message); err != nil {
		s.errors = append(s.errors, err)
		s.log().Error("problem sending message", "error", err)

		// increment metric
		s.sendErrors.Inc()

		// delivery may succeed later, so ask the client to retry
		return errDeliveryFailed
	}

	// increment metric
	s.emailTotal.Inc()

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
