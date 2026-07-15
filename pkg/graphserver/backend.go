package graphserver

import (
	"fmt"
	"log/slog"
	"net"
	"slices"

	"github.com/andrewheberle/graph-smtpd/pkg/graphclient"
	"github.com/emersion/go-smtp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Backend struct {
	client          *graphclient.Client
	saveToSentItems bool
	logger          *slog.Logger
	allowedSenders  []string
	allowedSources  []string

	reg prometheus.Registerer

	// metrics
	emailTotal prometheus.Counter
	sendErrors prometheus.Counter
	sendDenied prometheus.Counter
}

// NewGraphBackend sets up a new server
func NewGraphBackend(clientId, tenantId, secret string, opts ...BackendOption) (*Backend, error) {
	return newbackend(clientId, tenantId, secret, opts...)
}

func newbackend(clientId, tenantId, secret string, opts ...BackendOption) (*Backend, error) {
	b := new(Backend)

	// apply options
	for _, o := range opts {
		o(b)
	}

	// create graph client
	client, err := graphclient.NewClient(tenantId, clientId, secret)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	b.client = client

	// set defaults
	if b.allowedSenders == nil {
		b.allowedSenders = make([]string, 0)
	}

	if b.allowedSources == nil {
		b.allowedSources = make([]string, 0)
	}

	// set up metrics
	b.emailTotal = promauto.With(b.reg).NewCounter(
		prometheus.CounterOpts{
			Name: "graph_smtpd_email_total",
			Help: "Total number of emails accepted by SMTP",
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
	if len(b.allowedSources) > 0 {
		if addr, _, err := net.SplitHostPort(c.Conn().RemoteAddr().String()); err == nil {
			if _, found := slices.BinarySearch(b.allowedSources, addr); !found {
				b.sendDenied.Inc()
				return nil, fmt.Errorf("source not allowed")
			}
		}
	}

	// increment total metric
	b.emailTotal.Inc()

	// set up new session
	s := &Session{
		client:          b.client,
		allowedSenders:  b.allowedSenders,
		saveToSentItems: b.saveToSentItems,
		helo:            c.Hostname(),
		remote:          c.Conn().RemoteAddr().String(),
		errors:          make([]error, 0),
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

func WithAllowedSenders(senders []string) BackendOption {
	return func(b *Backend) {
		if senders == nil {
			b.allowedSenders = make([]string, 0)
		} else {
			// make sure it's sorted
			slices.Sort(senders)
			b.allowedSenders = senders
		}
	}
}

func WithAllowedSources(sources []string) BackendOption {
	return func(b *Backend) {
		if sources == nil {
			b.allowedSources = make([]string, 0)
		} else {
			// make sure it's sorted
			slices.Sort(sources)
			b.allowedSources = sources
		}
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
