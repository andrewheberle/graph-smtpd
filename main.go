package main

import (
	"context"
	"crypto/tls"
	"log/slog"
	"os"

	"github.com/andrewheberle/graph-smtpd/pkg/graphserver"
	"github.com/cloudflare/certinel/fswatcher"
	"github.com/emersion/go-smtp"
	"github.com/oklog/run"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func main() {
	// general options
	pflag.Bool("debug", false, "Enable debug mode")
	pflag.String("config", "", "Configuration file")

	// SMTP options
	pflag.String("addr", "localhost:2525", "Service listen address")
	pflag.String("domain", "localhost", "Service domain/hostname")
	pflag.Int("recipients", 10, "Maximum message recipients")
	pflag.Int64("max", 1024*1024, "Maximum message size in bytes")
	pflag.Bool("sentitems", false, "Save to sent items in senders mailbox")

	// Access controls
	pflag.StringSlice("senders", []string{}, "List of allowed senders")
	pflag.StringSlice("sources", []string{}, "Source IP addresses allowed to relay")

	// TLS options
	pflag.String("cert", "", "TLS certificate for STARTTLS")
	pflag.String("key", "", "TLS key for STARTTLS")

	// Entra ID options
	pflag.String("clientid", "", "App Registration Client/Application ID")
	pflag.String("tenantid", "", "App Registration Tenant ID")
	pflag.String("secret", "", "App Registration Client Secret")
	pflag.Parse()

	// set up logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// viper setup
	viper.SetEnvPrefix("smtpd")
	viper.AutomaticEnv()
	viper.BindPFlags(pflag.CommandLine)

	// load config file
	config := viper.GetString("config")
	if config != "" {
		viper.SetConfigFile(config)
	} else {
		viper.SetConfigName("config")
		viper.AddConfigPath(".")
	}
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			if config != "" {
				logger.Error("config file not found", "error", err, "config", config)
				os.Exit(1)
			} else {
				logger.Info("running without config")
			}
		} else {
			logger.Error("config file was invalid", "error", err, "config", viper.ConfigFileUsed())
			os.Exit(1)
		}
	} else {
		logger.Error("config file loaded", "config", viper.ConfigFileUsed())
	}

	// set backend options
	opts := []graphserver.BackendOption{
		graphserver.WithAllowedSenders(viper.GetStringSlice("senders")),
		graphserver.WithAllowedSources(viper.GetStringSlice("sources")),
		graphserver.WithSaveToSentItems(viper.GetBool("sentitems")),
		graphserver.WithLogger(logger),
	}

	// create backend
	be, err := graphserver.NewGraphBackend(viper.GetString("clientid"), viper.GetString("tenantid"), viper.GetString("secret"), opts...)
	if err != nil {
		slog.Error("error setting up backend", "error", err)
		os.Exit(1)
	}

	logger.Info("graph backend created")

	// set up server
	s := smtp.NewServer(be)
	s.Addr = viper.GetString("addr")
	s.Domain = viper.GetString("domain")
	s.MaxRecipients = viper.GetInt("recipients")
	s.MaxMessageBytes = viper.GetInt64("max")

	// set up run group
	g := run.Group{}

	if viper.GetString("cert") != "" && viper.GetString("key") != "" {
		ctx, cancel := context.WithCancel(context.Background())

		certinel, err := fswatcher.New(viper.GetString("cert"), viper.GetString("key"))
		if err != nil {
			logger.Error("could not set up certinel", "error", err, "cert", viper.GetString("cert"), "key", viper.GetString("key"))
			os.Exit(1)
		}

		// add certinel
		g.Add(func() error {
			logger.Info("starting up", "from", "certificate watcher", "cert", viper.GetString("cert"), "key", viper.GetString("key"))
			return certinel.Start(ctx)
		}, func(err error) {
			if err != nil {
				logger.Error("error on exit", "from", "certificate watcher", "error", err)
			}
			cancel()
		})

		// set up certiifcate watching for server
		s.TLSConfig = &tls.Config{
			GetCertificate: certinel.GetCertificate,
		}
	}

	// add SMTP server
	g.Add(func() error {
		logger.Info("starting up", "from", "SMTP server", "addr", viper.GetString("addr"), "domain", viper.GetString("domain"))
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
