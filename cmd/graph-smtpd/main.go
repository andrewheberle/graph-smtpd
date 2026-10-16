package main

import (
	"context"
	"crypto/tls"
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
	f.Bool("sentitems", false, "Save to sent items in senders mailbox")

	// Access controls
	f.StringSlice("senders", []string{}, "List of allowed senders")
	f.StringSlice("sources", []string{}, "Source IP addresses allowed to relay")

	// TLS options
	f.String("cert", "", "TLS certificate for STARTTLS")
	f.String("key", "", "TLS key for STARTTLS")

	// Entra ID options
	f.String("clientid", "", "App Registration Client/Application ID")
	f.String("tenantid", "", "App Registration Tenant ID")
	f.String("secret", "", "App Registration Client Secret")

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
	k, err := configger.LoadConfig(f, configger.WithEnvPrefix("SMTPD"))
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
		graphserver.WithLogger(logger),
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

	// get credentials, falling back to secret_file when no secret is set
	tenantID, clientID, secret, err := cli.Credentials(k)
	if err != nil {
		// not a fatal error at this point
		logger.Warn("could not load secret", "error", err)
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
	s := smtp.NewServer(be)
	s.Addr = k.String("addr")
	s.Domain = k.String("domain")
	s.MaxRecipients = k.Int("recipients")
	s.MaxMessageBytes = k.Int64("max")

	if k.String("cert") != "" && k.String("key") != "" {
		ctx, cancel := context.WithCancel(context.Background())

		certinel, err := fswatcher.New(k.String("cert"), k.String("key"))
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

		// set up certificate watching for server
		s.TLSConfig = &tls.Config{
			GetCertificate: certinel.GetCertificate,
		}
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
