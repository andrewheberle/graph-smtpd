package graphserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/andrewheberle/graph-smtpd/pkg/sendmail"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/prometheus/client_golang/prometheus"
)

// Message outcomes, logged as "status" on each message line.
const (
	statusSent      = "sent"      // accepted by Graph
	statusRejected  = "rejected"  // refused permanently (5xx)
	statusDeferred  = "deferred"  // refused temporarily (4xx), so the client should retry
	statusAbandoned = "abandoned" // the client gave up before completing DATA
)

// replyQueued is the reply go-smtp sends when Data succeeds.
const replyQueued = "250 2.0.0 OK: queued"

// Session handles one SMTP session. Each message attempt is logged as a
// single "message" line once its outcome is known, so the log shows what
// happened to every message the relay was offered and why.
type Session struct {
	conn            *smtp.Conn
	connID          string
	tls             bool
	mailer          Mailer
	saveToSentItems bool
	sendTimeout     time.Duration
	logger          *slog.Logger
	policy          *relayPolicy
	sourceDenied    bool
	helo            string
	remote          string
	upgraded        func(*smtp.Conn, string)

	// SMTP AUTH: the accounts clients may log in as, and the user this
	// session logged in as
	users        *Users
	user         string
	authDelay    time.Duration
	authFailures prometheus.Counter

	// counts of message outcomes, for the disconnect line
	outcomes map[string]int
	seq      int

	// the current message
	from      string
	rcpt      []string
	id        string
	started   time.Time
	size      int64
	message   *sendmail.Message
	inMessage bool
	finished  bool

	emailTotal prometheus.Counter
	sendErrors prometheus.Counter
	sendDenied prometheus.Counter
}

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	s.seq++
	s.inMessage = true
	s.finished = false
	s.id = fmt.Sprintf("%s.%d", s.connID, s.seq)
	s.from = from
	s.started = time.Now()

	// check that the client may relay: it has logged in, or its address is
	// allowed
	if s.sourceDenied && s.user == "" {
		reason := errors.New("client address is not in the allowed sources")
		if s.users != nil {
			reason = errors.New("client is not authenticated and its address is not in the allowed sources")
		}
		s.sendDenied.Inc()
		s.finish(statusRejected, errSourceDenied, reason)

		return errSourceDenied
	}

	// check that sender is allowed
	if !s.policy.allowSender(from) {
		s.sendDenied.Inc()
		s.finish(statusRejected, errSenderDenied, errors.New("sender is not in the allowed senders"))

		return errSenderDenied
	}

	return nil
}

func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	s.rcpt = append(s.rcpt, to)

	return nil
}

func (s *Session) Data(r io.Reader) error {
	// build message from incoming data, counting the whole message including
	// anything the parser does not read
	counted := &countingReader{r: r}
	env := sendmail.Envelope{From: s.from, Rcpt: s.rcpt}
	message, err := sendmail.FromMIME(counted, env, sendmail.WithSaveToSentItems(s.saveToSentItems))
	_, _ = io.Copy(io.Discard, counted)
	s.size = counted.n

	if err != nil {
		// increment metric
		s.sendErrors.Inc()

		// the message itself is unusable, so retrying will not help
		reply := &smtp.SMTPError{
			Code:         554,
			EnhancedCode: smtp.EnhancedCode{5, 6, 0},
			Message:      "message could not be processed: " + err.Error(),
		}
		s.finish(statusRejected, reply, err)

		return reply
	}
	s.message = message

	// send it, within the send timeout if one is set
	ctx := context.Background()
	if s.sendTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.sendTimeout)
		defer cancel()
	}

	if err := s.mailer.Send(ctx, message); err != nil {
		// increment metric
		s.sendErrors.Inc()

		// ask the client to retry unless Graph rejected the message outright
		reply := deliveryError(err)
		status := statusRejected
		var smtpErr *smtp.SMTPError
		if errors.As(reply, &smtpErr) && smtpErr.Code < 500 {
			status = statusDeferred
		}
		s.finish(status, reply, err)

		return reply
	}

	// increment metric
	s.emailTotal.Inc()

	s.finish(statusSent, nil, nil)

	return nil
}

// AuthMechanisms returns the SASL mechanisms clients may use to log in:
// none unless users are configured. go-smtp only offers and accepts them on
// a TLS connection.
func (s *Session) AuthMechanisms() []string {
	if s.users == nil {
		return nil
	}

	return []string{sasl.Plain, sasl.Login}
}

// Auth starts a login with mechanism mech.
func (s *Session) Auth(mech string) (sasl.Server, error) {
	switch mech {
	case sasl.Plain:
		return sasl.NewPlainServer(func(identity, username, password string) error {
			// a client may not log in as one user to act as another
			if identity != "" && identity != username {
				return s.loginFailed(mech, username)
			}
			return s.login(mech, username, password)
		}), nil
	case sasl.Login:
		return newLoginServer(func(username, password string) error {
			return s.login(mech, username, password)
		}), nil
	default:
		return nil, smtp.ErrAuthUnknownMechanism
	}
}

// login checks a username and password.
func (s *Session) login(mech, username, password string) error {
	if !s.users.authenticate(username, password) {
		return s.loginFailed(mech, username)
	}

	s.user = username
	s.logger.Info("auth", append(s.connAttrs(), "mechanism", mech)...)

	return nil
}

// loginFailed logs and counts a failed login, and waits before replying to
// slow down password guessing.
func (s *Session) loginFailed(mech, username string) error {
	if s.authFailures != nil {
		s.authFailures.Inc()
	}
	s.logger.Warn("auth failed", append(s.connAttrs(), "mechanism", mech, "username", username)...)
	time.Sleep(s.authDelay)

	return errAuthFailed
}

// Reset discards the current message, so the next message in the session
// starts with a clean envelope.
func (s *Session) Reset() {
	s.abandon()

	s.from = ""
	s.rcpt = nil
	s.id = ""
	s.size = 0
	s.message = nil
	s.inMessage = false
	s.finished = false
}

// Logout is called when the connection closes, and when the client upgrades
// the connection with STARTTLS, in which case go-smtp starts a new session
// on the upgraded connection.
func (s *Session) Logout() error {
	s.abandon()

	if s.conn != nil {
		if _, nowTLS := s.conn.TLSConnectionState(); nowTLS && !s.tls {
			// the connection carries on in a new session, under the same ID,
			// which logs the upgrade
			if s.upgraded != nil {
				s.upgraded(s.conn, s.connID)
			}

			return nil
		}
	}

	attrs := append(s.connAttrs(), "messages", s.seq)
	for _, status := range []string{statusSent, statusRejected, statusDeferred, statusAbandoned} {
		if n := s.outcomes[status]; n > 0 {
			attrs = append(attrs, status, n)
		}
	}
	s.logger.Info("disconnect", attrs...)

	return nil
}

// abandon logs the current message if the client gave up on it before its
// outcome was known.
func (s *Session) abandon() {
	if s.inMessage && !s.finished {
		s.finish(statusAbandoned, nil, nil)
	}
}

// finish logs the outcome of the current message: the reply the client was
// sent and, for a failure, the underlying reason.
func (s *Session) finish(status string, reply error, reason error) {
	s.finished = true
	if s.outcomes == nil {
		s.outcomes = make(map[string]int)
	}
	s.outcomes[status]++

	rcpt := s.rcpt
	if rcpt == nil {
		rcpt = []string{}
	}
	attrs := append(s.connAttrs(),
		"id", s.id,
		"from", s.from,
		"rcpt", rcpt,
	)
	if s.size > 0 {
		attrs = append(attrs, "size", s.size)
	}
	if s.message != nil && s.message.MessageID() != "" {
		attrs = append(attrs, "message_id", s.message.MessageID())
	}
	attrs = append(attrs, "status", status)
	switch {
	case status == statusSent:
		attrs = append(attrs, "reply", replyQueued)
	case reply != nil:
		attrs = append(attrs, "reply", replyText(reply))
	}
	if reason != nil {
		attrs = append(attrs, "reason", reason.Error())
	}
	attrs = append(attrs, "duration", time.Since(s.started).Round(time.Millisecond).String())

	level := slog.LevelInfo
	if status == statusRejected || status == statusDeferred {
		level = slog.LevelWarn
	}
	s.logger.Log(context.Background(), level, "message", attrs...)

	// header details can be personal, so they are only logged for debugging
	if s.message != nil {
		s.logger.Debug("message details", "conn", s.connID, "id", s.id, "message", s.message)
	}
}

// connAttrs returns the attributes that identify the connection, and the
// user once the client has logged in.
func (s *Session) connAttrs() []any {
	attrs := []any{"conn", s.connID, "client", s.remote, "helo", s.helo, "tls", s.tls}
	if s.user != "" {
		attrs = append(attrs, "user", s.user)
	}

	return attrs
}

// replyText formats an SMTP error as the reply line the client received.
func replyText(err error) string {
	var smtpErr *smtp.SMTPError
	if !errors.As(err, &smtpErr) {
		return err.Error()
	}

	return fmt.Sprintf("%d %d.%d.%d %s", smtpErr.Code,
		smtpErr.EnhancedCode[0], smtpErr.EnhancedCode[1], smtpErr.EnhancedCode[2], smtpErr.Message)
}

// countingReader counts the bytes read through it.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)

	return n, err
}
