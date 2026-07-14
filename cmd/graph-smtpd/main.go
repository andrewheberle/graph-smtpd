package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/andrewheberle/graph-smtpd/pkg/graphserver"
	"github.com/andrewheberle/redacted-string"
	"github.com/cloudflare/certinel/fswatcher"
	"github.com/emersion/go-smtp"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/v2"
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

	k := koanf.New(".")

	// set up logger
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	// load any config file
	if config, err := f.GetString("config"); err != nil {
		logger.Error("error getting flag value", "error", err)
		os.Exit(1)
	} else if config != "" {
		if err := k.Load(file.Provider(config), yaml.Parser()); err != nil {
			logger.Error("error loading configuiration file", "error", err)
			os.Exit(1)
		}
	}

	// Load env vars
	if err := k.Load(env.Provider(".", env.Opt{
		Prefix: "SMTPD_",
		TransformFunc: func(k, v string) (string, any) {
			// Transform the key.
			k = strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(k, "SMTPD_")), "_", ".")

			// Transform values with commas into slices
			if strings.Contains(v, ",") {
				return k, strings.Split(v, ",")
			}

			return k, v
		},
	}), nil); err != nil {
		logger.Error("error reading env vars", "error", err)
		os.Exit(1)
	}

	// Load command line options
	if err := k.Load(posflag.Provider(f, ".", k), nil); err != nil {
		logger.Error("error reading command line", "error", err)
		os.Exit(1)
	}

	// set backend options
	opts := []graphserver.BackendOption{
		graphserver.WithAllowedSenders(k.Strings("senders")),
		graphserver.WithAllowedSources(k.Strings("sources")),
		graphserver.WithSaveToSentItems(k.Bool("sentitems")),
		graphserver.WithLogger(logger),
	}

	// set up metrics
	metrics := k.String("metrics")
	if metrics != "" {
		reg := prometheus.NewRegistry()
		reg.MustRegister(
			collectors.NewGoCollector(),
			collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		)
		http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
		opts = append(opts, graphserver.WithPrometheusRegistry(reg))
	}

	// check secret was set, otherwise try the _FILE variation
	if k.String("secret") == "" && k.String("secret_file") != "" {
		// read from SMTPD_SECRET_FILE
		b, err := os.ReadFile(k.String("secret_file"))
		if err == nil {
			// if that worked then set SMTPD_SECRET
			k.Set("secret", strings.TrimSpace(string(b)))
		} else {
			// not a fatal error at this point
			logger.Warn("could not read", "secret_file", k.String("secret_file"), "error", err)
		}
	}

	// create backend
	be, err := graphserver.NewGraphBackend(k.String("clientid"), k.String("tenantid"), k.String("secret"), opts...)
	if err != nil {
		logger.Error("error setting up backend",
			"error", err,
			"clientid", k.String("clientid"),
			"tenantid", k.String("tenantid"),
			"secret", redacted.Redact(k.String("secret")),
		)
		os.Exit(1)
	}

	logger.Info("graph backend created")

	// set up server
	s := smtp.NewServer(be)
	s.Addr = k.String("addr")
	s.Domain = k.String("domain")
	s.MaxRecipients = k.Int("recipients")
	s.MaxMessageBytes = k.Int64("max")

	// set up run group
	g := run.Group{}

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

	// set up metrics http listener if set
	if metrics != "" {
		g.Add(func() error {
			logger.Info("starting up", "from", "metrics", "addr", metrics)
			return http.ListenAndServe(metrics, nil)
		}, func(err error) {
			if err != nil {
				logger.Error("error on exit", "from", "metrics", "error", err)
			}
			s.Close()
		})
	}

	// add SMTP server
	g.Add(func() error {
		logger.Info("starting up", "from", "SMTP server", "addr", k.String("addr"), "domain", k.String("domain"))
		return s.ListenAndServe()
	}, func(err error) {
		if err != nil {
			logger.Error("error on exit", "from", "SMTP server", "error", err)
		}
		s.Close()
	})

	logger.Info("starting components")

	if err := g.Run(); err != nil {
		logger.Error("run group error", "error", err)
		os.Exit(1)
	}
}
