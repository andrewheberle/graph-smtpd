package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/andrewheberle/configger"
	"github.com/andrewheberle/graph-smtpd/internal/cli"
	"github.com/andrewheberle/graph-smtpd/pkg/sendmail"
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
	f.String("config", "", "Configuration file")
	f.Bool("debug", false, "Enable debugging")
	f.Bool("quiet", false, "Enable quiet mode (no logging)")
	f.Bool("version", false, "Display version and exit")

	// Entra ID options
	f.String("clientid", "", "App Registration Client/Application ID")
	f.String("tenantid", "", "App Registration Tenant ID")
	f.String("secret", "", "App Registration Client Secret")

	// sending options
	f.Bool("sentitems", false, "Save to sent items in senders mailbox")

	// parse command line
	if err := f.Parse(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing command line flags: %s\n", err)
		os.Exit(1)
	}

	// handle if version was requested
	if version, err := f.GetBool("version"); err == nil && version {
		fmt.Printf("graph-sendmail %s\n", Version)
		os.Exit(0)
	}

	// set up logger
	logLevel := new(slog.LevelVar)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// discard time
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}

			return a
		},
	}))

	// load config file, env vars and command line options
	k, err := loadConfig(f)
	if err != nil {
		logger.Error("error loading configuration", "error", err)
		os.Exit(1)
	}

	// silence logger ig set in quiet mode
	if k.Bool("quiet") {
		logger = slog.New(slog.DiscardHandler)
	} else {
		if k.Bool("debug") {
			logLevel.Set(slog.LevelDebug)
		}
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
		slog.Error("could not create graph mailer", "error", err)
		os.Exit(1)
	}

	// build message from stdin, using headers in place of an envelope
	message, err := sendmail.FromMIME(os.Stdin, sendmail.Envelope{}, sendmail.WithSaveToSentItems(k.Bool("sentitems")))
	if err != nil {
		slog.Error("unable to read message", "error", err)
		os.Exit(1)
	}

	// add context to logger
	logger = logger.With("message", message)

	// send email
	if err := mailer.Send(context.Background(), message); err != nil {
		logger.Error("error sending email", "error", err)
		os.Exit(1)
	}

	logger.Info("message sent")
}

// loadConfig loads configuration from the config file, SENDMAIL_
// environment variables and the parsed command line flags in f.
func loadConfig(f *pflag.FlagSet) (configger.Config, error) {
	return configger.LoadConfig(f, configger.WithEnvPrefix("SENDMAIL"))
}
