package graphserver

import (
	"context"
	"errors"
	"log/slog"

	"github.com/andrewheberle/graph-smtpd/pkg/sendmail"
	"github.com/emersion/go-smtp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Mailer delivers a Message.
type Mailer interface {
	Send(ctx context.Context, m *sendmail.Message) error
}

type Backend struct {
	mailer          Mailer
	saveToSentItems bool
	logger          *slog.Logger
	allowedSenders  []string
	allowedSources  []string
	policy          *relayPolicy

	reg prometheus.Registerer

	// metrics
	emailTotal prometheus.Counter
	sendErrors prometheus.Counter
	sendDenied prometheus.Counter
}

// NewBackend sets up a new SMTP backend that delivers each message using
// mailer.
func NewBackend(mailer Mailer, opts ...BackendOption) (*Backend, error) {
	if mailer == nil {
		return nil, errors.New("mailer must not be nil")
	}

	b := &Backend{mailer: mailer}

	// apply options
	for _, o := range opts {
		o(b)
	}

	policy, err := newRelayPolicy(b.allowedSenders, b.allowedSources)
	if err != nil {
		return nil, err
	}
	b.policy = policy

	// set up metrics
	b.emailTotal = promauto.With(b.reg).NewCounter(
		prometheus.CounterOpts{
			Name: "graph_smtpd_email_total",
			Help: "Total number of emails accepted by SMTP and sent",
		},
	)
	b.sendErrors = promauto.With(b.reg).NewCounter(
		prometheus.CounterOpts{
			Name: "graph_smtpd_email_errors_total",
			Help: "Total number of send errors",
		},
	)
	b.sendDenied = promauto.With(b.reg).NewCounter(
		prometheus.CounterOpts{
			Name: "graph_smtpd_email_denied_total",
			Help: "Total number of emails denied",
		},
	)

	return b, nil
}

// NewSession is called after client greeting (EHLO, HELO).
func (b *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	// Check if IP is allowed
	if !b.policy.allowSource(c.Conn().RemoteAddr()) {
		b.sendDenied.Inc()
		return nil, errSourceDenied
	}

	// set up new session
	s := &Session{
		mailer:          b.mailer,
		policy:          b.policy,
		saveToSentItems: b.saveToSentItems,
		helo:            c.Hostname(),
		remote:          c.Conn().RemoteAddr().String(),
		errors:          make([]error, 0),
		emailTotal:      b.emailTotal,
		sendErrors:      b.sendErrors,
		sendDenied:      b.sendDenied,
	}

	if b.logger == nil {
		s.logger = slog.New(slog.DiscardHandler)
	} else {
		s.logger = b.logger
	}

	return s, nil
}

type BackendOption func(*Backend)

func WithSaveToSentItems(save bool) BackendOption {
	return func(b *Backend) {
		b.saveToSentItems = save
	}
}

// WithAllowedSenders limits which sender addresses may send. An empty list
// allows any sender.
func WithAllowedSenders(senders []string) BackendOption {
	return func(b *Backend) {
		b.allowedSenders = senders
	}
}

// WithAllowedSources limits which client IP addresses may relay. An empty
// list allows any source.
func WithAllowedSources(sources []string) BackendOption {
	return func(b *Backend) {
		b.allowedSources = sources
	}
}

func WithLogger(logger *slog.Logger) BackendOption {
	return func(b *Backend) {
		b.logger = logger
	}
}

func WithPrometheusRegistry(reg *prometheus.Registry) BackendOption {
	return func(b *Backend) {
		b.reg = reg
	}
}
