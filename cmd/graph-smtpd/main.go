package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/andrewheberle/configger"
	"github.com/andrewheberle/graph-smtpd/internal/cli"
	"github.com/andrewheberle/graph-smtpd/pkg/graphserver"
	"github.com/andrewheberle/graph-smtpd/pkg/sendmail"
	"github.com/andrewheberle/redacted-string"
	"github.com/cloudflare/certinel/fswatcher"
	"github.com/emersion/go-smtp"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/pflag"
)

var Version = "dev"

func main() {
	// set up flagset
	f := pflag.NewFlagSet("config", pflag.ContinueOnError)
	f.Usage = func() {
		fmt.Println(f.FlagUsages())
		os.Exit(0)
	}

	// general options
	f.Bool("debug", false, "Enable debug mode")
	f.String("config", "", "Configuration file")
	f.Bool("version", false, "Display version and exit")

	// SMTP options
	f.String("addr", "localhost:2525", "Service listen address")
	f.String("domain", "localhost", "Service domain/hostname")
	f.Int("recipients", 10, "Maximum message recipients")
	f.Int64("max", 1024*1024, "Maximum message size in bytes")
	f.Duration("conntimeout", 5*time.Minute, "Time allowed for each read or write on a client connection (0 for no limit)")
	f.Bool("sentitems", false, "Save to sent items in senders mailbox")
	f.Duration("timeout", graphserver.DefaultSendTimeout, "Time allowed to send each message via Graph (0 for no limit)")

	// Access controls
	f.StringSlice("senders", []string{}, "List of allowed senders (case-insensitive)")
	f.StringSlice("sources", []string{}, "Source IP addresses or CIDR ranges allowed to relay")

	// TLS options
	f.String("cert", "", "TLS certificate for STARTTLS")
	f.String("key", "", "TLS key for STARTTLS")

	// SMTP AUTH
	f.String("users", "", "File of SMTP AUTH users in htpasswd format, with bcrypt hashes (requires --cert and --key)")

	// Entra ID options
	f.String("clientid", "", "App Registration Client/Application ID")
	f.String("tenantid", "", "App Registration Tenant ID")
	f.String("secret", "", "App Registration Client Secret")
	f.String("secretfile", "", "App Registration Client Secret (from file)")

	// metrics
	f.String("metrics", "", "Listen address for metrics")

	// parse command line
	if err := f.Parse(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing command line flags: %s\n", err)
		os.Exit(1)
	}

	// handle if version was requested
	if version, err := f.GetBool("version"); err == nil && version {
		fmt.Printf("graph-smtpd %s\n", Version)
		os.Exit(0)
	}

	// set up logger
	logLevel := new(slog.LevelVar)
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// load config file, env vars and command line options
	k, err := loadConfig(f)
	if err != nil {
		logger.Error("error loading configuration", "error", err)
		os.Exit(1)
	}

	if k.Bool("debug") {
		logLevel.Set(slog.LevelDebug)
	}

	// set backend options
	opts := []graphserver.BackendOption{
		graphserver.WithAllowedSenders(k.Strings("senders")),
		graphserver.WithAllowedSources(k.Strings("sources")),
		graphserver.WithSaveToSentItems(k.Bool("sentitems")),
		graphserver.WithSendTimeout(k.Duration("timeout")),
		graphserver.WithLogger(logger),
	}

	// load SMTP AUTH users
	users, err := loadUsers(k)
	if err != nil {
		logger.Error("error loading users", "error", err)
		os.Exit(1)
	}
	if users != nil {
		opts = append(opts, graphserver.WithUsers(users))
		logger.Info("smtp auth enabled", "users", users.Len(), "file", k.String("users"))
	}

	// set up run group
	g := run.Group{}

	// set up metrics
	metrics := k.String("metrics")
	if metrics != "" {
		reg := prometheus.NewRegistry()
		reg.MustRegister(
			collectors.NewGoCollector(),
			collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		)

		opts = append(opts, graphserver.WithPrometheusRegistry(reg))

		// set up metrics http listener
		h := http.NewServeMux()
		h.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
		s := &http.Server{
			Addr:    metrics,
			Handler: h,
		}

		g.Add(func() error {
			logger.Info("starting up", "from", "metrics", "addr", metrics)
			return s.ListenAndServe()
		}, func(err error) {
			if err != nil {
				logger.Error("error on exit", "from", "metrics", "error", err)
			}
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
				defer cancel()

				if err := s.Shutdown(ctx); err != nil {
					logger.Error("error when shutting down", "from", "metrics", "error", err)
				}
			}()
		})
	}

	// get credentials, falling back to a secret file when no secret is set
	tenantID, clientID, secret, err := cli.Credentials(k)
	if err != nil {
		logger.Error("could not load secret", "error", err)
		os.Exit(1)
	}

	// create graph mailer
	mailer, err := sendmail.NewGraphMailer(tenantID, clientID, secret)
	if err != nil {
		logger.Error("error setting up graph mailer",
			"error", err,
			"clientid", clientID,
			"tenantid", tenantID,
			"secret", redacted.Redact(secret),
		)
		os.Exit(1)
	}

	// create backend
	be, err := graphserver.NewBackend(mailer, opts...)
	if err != nil {
		logger.Error("error setting up backend", "error", err)
		os.Exit(1)
	}

	logger.Info("graph backend created",
		"clientid", clientID,
		"tenantid", tenantID,
		"secret", redacted.Redact(secret),
	)

	// set up server
	s := newServer(be, k, logger)

	if k.String("cert") != "" && k.String("key") != "" {
		ctx, cancel := context.WithCancel(context.Background())

		certinel, tlsConfig, err := newTLS(k.String("cert"), k.String("key"))
		if err != nil {
			logger.Error("could not set up certinel", "error", err, "cert", k.String("cert"), "key", k.String("key"))
			os.Exit(1)
		}

		// add certinel
		g.Add(func() error {
			logger.Info("starting up", "from", "certificate watcher", "cert", k.String("cert"), "key", k.String("key"))
			return certinel.Start(ctx)
		}, func(err error) {
			if err != nil {
				logger.Error("error on exit", "from", "certificate watcher", "error", err)
			}
			cancel()
		})

		// serve the watched certificate for STARTTLS
		s.TLSConfig = tlsConfig
	}

	// add SMTP server
	g.Add(func() error {
		logger.Info("starting up", "from", "SMTP server", "addr", k.String("addr"), "domain", k.String("domain"))
		return s.ListenAndServe()
	}, func(err error) {
		if err != nil {
			logger.Error("error on exit", "from", "SMTP server", "error", err)
		}
		if err := s.Close(); err != nil {
			logger.Error("error when shutting down", "from", "SMTP server", "error", err)
		}
	})

	logger.Info("starting components")

	if err := g.Run(); err != nil {
		logger.Error("run group error", "error", err)
		os.Exit(1)
	}
}

// loadConfig loads configuration from the config file, SMTPD_ environment
// variables and the parsed command line flags in f.
func loadConfig(f *pflag.FlagSet) (configger.Config, error) {
	return configger.LoadConfig(f, configger.WithEnvPrefix("SMTPD"))
}

// newServer creates the SMTP server for be from the configuration in k,
// logging its internal errors to logger.
func newServer(be smtp.Backend, k configger.Config, logger *slog.Logger) *smtp.Server {
	s := smtp.NewServer(be)
	s.ErrorLog = graphserver.NewErrorLog(logger)
	s.Addr = k.String("addr")
	s.Domain = k.String("domain")
	s.MaxRecipients = k.Int("recipients")
	s.MaxMessageBytes = k.Int64("max")
	s.ReadTimeout = k.Duration("conntimeout")
	s.WriteTimeout = k.Duration("conntimeout")

	return s
}

// newTLS creates a watcher for the certificate and key files, and a TLS
// config that always serves the watcher's current certificate, so a renewed
// certificate is used without a restart. The watcher must be started for
// changes to be picked up.
func newTLS(cert, key string) (*fswatcher.Sentinel, *tls.Config, error) {
	sentinel, err := fswatcher.New(cert, key)
	if err != nil {
		return nil, nil, err
	}

	return sentinel, &tls.Config{GetCertificate: sentinel.GetCertificate}, nil
}

// loadUsers loads the SMTP AUTH users file named by the "users" option, or
// returns nil if there is none. AUTH is only allowed over TLS, so a users
// file needs a certificate and key.
func loadUsers(k configger.Config) (*graphserver.Users, error) {
	path := k.String("users")
	if path == "" {
		return nil, nil
	}
	if k.String("cert") == "" || k.String("key") == "" {
		return nil, errors.New("--users needs --cert and --key, as SMTP AUTH is only allowed over TLS")
	}

	return graphserver.LoadUsers(path)
}
