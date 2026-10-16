package graphserver

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/andrewheberle/graph-smtpd/pkg/sendmail"
	"github.com/emersion/go-smtp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// DefaultSendTimeout is how long a Mailer has to deliver a message, unless
// changed with WithSendTimeout.
const DefaultSendTimeout = 30 * time.Second

// errDeliveryFailed is returned when the Mailer fails temporarily. It is 451
// 4.4.1, so the client retries rather than dropping the message; the cause
// is logged rather than returned to the client.
var errDeliveryFailed = &smtp.SMTPError{
	Code:         451,
	EnhancedCode: smtp.EnhancedCode{4, 4, 1},
	Message:      "temporary failure delivering message, try again later",
}

// deliveryError returns the SMTP reply for a Mailer failure: permanent when
// Graph rejected the message, so the client stops retrying, and temporary
// otherwise. Graph's error code is included, but not its message.
func deliveryError(err error) error {
	var rejected *sendmail.RejectedError
	if !errors.As(err, &rejected) {
		return errDeliveryFailed
	}

	reply := &smtp.SMTPError{
		Code:         554,
		EnhancedCode: smtp.EnhancedCode{5, 0, 0},
		Message:      "delivery refused by Microsoft Graph",
	}
	switch rejected.StatusCode {
	case http.StatusForbidden:
		// the app may not send as this mailbox
		reply.Code, reply.EnhancedCode = 550, smtp.EnhancedCode{5, 7, 1}
	case http.StatusNotFound:
		// the mailbox does not exist
		reply.Code, reply.EnhancedCode = 550, smtp.EnhancedCode{5, 1, 1}
	case http.StatusRequestEntityTooLarge:
		reply.Code, reply.EnhancedCode = 552, smtp.EnhancedCode{5, 3, 4}
	}
	if rejected.Code != "" {
		reply.Message += " (" + rejected.Code + ")"
	}

	return reply
}

// errAuthFailed is the reply to a failed login.
var errAuthFailed = &smtp.SMTPError{
	Code:         535,
	EnhancedCode: smtp.EnhancedCode{5, 7, 8},
	Message:      "authentication failed",
}

// defaultAuthDelay is how long a failed login waits before its reply, to
// slow down password guessing.
const defaultAuthDelay = time.Second

// Mailer delivers a Message.
type Mailer interface {
	Send(ctx context.Context, m *sendmail.Message) error
}

type Backend struct {
	mailer          Mailer
	saveToSentItems bool
	sendTimeout     time.Duration
	logger          *slog.Logger
	allowedSenders  []string
	allowedSources  []string
	policy          *relayPolicy
	users           *Users
	authDelay       time.Duration

	reg prometheus.Registerer

	// connections upgraded with STARTTLS, waiting for their next session
	upgradesMu sync.Mutex
	upgrades   map[*smtp.Conn]upgrade

	// metrics
	emailTotal   prometheus.Counter
	sendErrors   prometheus.Counter
	sendDenied   prometheus.Counter
	authFailures prometheus.Counter
}

// NewBackend sets up a new SMTP backend that delivers each message using
// mailer.
func NewBackend(mailer Mailer, opts ...BackendOption) (*Backend, error) {
	if mailer == nil {
		return nil, errors.New("mailer must not be nil")
	}

	b := &Backend{mailer: mailer, sendTimeout: DefaultSendTimeout, authDelay: defaultAuthDelay}

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
	b.authFailures = promauto.With(b.reg).NewCounter(
		prometheus.CounterOpts{
			Name: "graph_smtpd_auth_failures_total",
			Help: "Total number of failed SMTP AUTH logins",
		},
	)

	return b, nil
}

// upgrade is a connection upgraded with STARTTLS whose new session has not
// started yet.
type upgrade struct {
	connID string
	at     time.Time
}

// upgradeExpiry is how long an upgraded connection's ID is kept waiting for
// the client's next EHLO.
const upgradeExpiry = time.Minute

// upgraded records that c was upgraded with STARTTLS, so its next session
// keeps the connection ID.
func (b *Backend) upgraded(c *smtp.Conn, connID string) {
	b.upgradesMu.Lock()
	defer b.upgradesMu.Unlock()

	if b.upgrades == nil {
		b.upgrades = make(map[*smtp.Conn]upgrade)
	}
	b.upgrades[c] = upgrade{connID: connID, at: time.Now()}
}

// resumed returns the connection ID of c if it was just upgraded with
// STARTTLS, dropping any upgrades whose client never sent EHLO again.
func (b *Backend) resumed(c *smtp.Conn) (string, bool) {
	b.upgradesMu.Lock()
	defer b.upgradesMu.Unlock()

	u, ok := b.upgrades[c]
	delete(b.upgrades, c)
	for conn, other := range b.upgrades {
		if time.Since(other.at) > upgradeExpiry {
			delete(b.upgrades, conn)
		}
	}

	return u.connID, ok
}

// sourceAllowed reports whether a client from addr may relay without
// logging in. With users configured, only the listed sources may.
func (b *Backend) sourceAllowed(addr net.Addr) bool {
	if b.users != nil && !b.policy.hasSources() {
		return false
	}

	return b.policy.allowSource(addr)
}

// newConnID returns a short random ID that ties together the log lines of
// one connection.
func newConnID() string {
	b := make([]byte, 5)
	_, _ = rand.Read(b)

	return hex.EncodeToString(b)
}

// NewSession is called after client greeting (EHLO, HELO), and again after
// STARTTLS.
//
// A session is created even for a source that may not relay, so the client
// gets a normal greeting and each message it tries is refused, and counted,
// once at MAIL FROM.
func (b *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	tlsState, isTLS := c.TLSConnectionState()
	connID, resumed := b.resumed(c)
	if !resumed {
		connID = newConnID()
	}

	s := &Session{
		conn:            c,
		connID:          connID,
		tls:             isTLS,
		upgraded:        b.upgraded,
		sourceDenied:    !b.sourceAllowed(c.Conn().RemoteAddr()),
		users:           b.users,
		authDelay:       b.authDelay,
		authFailures:    b.authFailures,
		mailer:          b.mailer,
		policy:          b.policy,
		saveToSentItems: b.saveToSentItems,
		sendTimeout:     b.sendTimeout,
		helo:            c.Hostname(),
		remote:          c.Conn().RemoteAddr().String(),
		emailTotal:      b.emailTotal,
		sendErrors:      b.sendErrors,
		sendDenied:      b.sendDenied,
	}

	if b.logger == nil {
		s.logger = slog.New(slog.DiscardHandler)
	} else {
		s.logger = b.logger
	}

	if resumed {
		s.logger.Info("starttls", append(s.connAttrs(), "tls_version", tls.VersionName(tlsState.Version))...)
	} else {
		s.logger.Info("connect", s.connAttrs()...)
	}

	return s, nil
}

type BackendOption func(*Backend)

func WithSaveToSentItems(save bool) BackendOption {
	return func(b *Backend) {
		b.saveToSentItems = save
	}
}

// WithSendTimeout sets how long the Mailer has to deliver each message
// before the client is told to retry. Zero or less means no timeout.
func WithSendTimeout(d time.Duration) BackendOption {
	return func(b *Backend) {
		b.sendTimeout = d
	}
}

// WithUsers lets clients log in with SMTP AUTH (PLAIN or LOGIN, over TLS
// only) as any of users. A client that has logged in may relay from any
// address; one that has not may only relay from an allowed source, and with
// users set, an empty list of allowed sources allows none.
func WithUsers(users *Users) BackendOption {
	return func(b *Backend) {
		b.users = users
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
