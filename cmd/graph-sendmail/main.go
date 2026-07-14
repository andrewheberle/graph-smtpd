package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/OfimaticSRL/parsemail"
	"github.com/andrewheberle/graph-smtpd/pkg/graphclient"
	"github.com/andrewheberle/graph-smtpd/pkg/sendmail"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/v2"
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

	k := koanf.New(".")

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
		Prefix: "SENDMAIL_",
		TransformFunc: func(k, v string) (string, any) {
			// Transform the key.
			k = strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(k, "SENDMAIL_")), "_", ".")

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

	// silence logger ig set in quiet mode
	if k.Bool("quiet") {
		logger = slog.New(slog.DiscardHandler)
	} else {
		if k.Bool("debug") {
			logLevel.Set(slog.LevelDebug)
		}
	}

	// create graph client
	client, err := graphclient.NewClient(k.String("tenantid"), k.String("clientId"), k.String("secret"))
	if err != nil {
		slog.Error("could not create graph client", "error", err)
		os.Exit(1)
	}

	// parse incoming message
	msg, err := parsemail.Parse(os.Stdin)
	if err != nil {
		slog.Error("unable to read message", "error", err)
		os.Exit(1)
	}

	// with debug enabled show whole message
	slog.Debug("message", "msg", msg)

	// grab headers and content
	header := msg.Header
	subject := header.Get("Subject")
	from := header.Get("From")
	to := header.Get("To")

	// message options
	opts := []sendmail.MessageOption{
		sendmail.WithAttachments(msg.Attachments),
		sendmail.WithSaveToSentItems(k.Bool("sentitems")),
	}

	// add context to logger
	logger = logger.With("from", from, "to", to, "subject", subject)

	// add Cc recipients
	if cc := header.Get("Cc"); cc != "" {
		logger = logger.With("cc", cc)
		opts = append(opts, sendmail.WithCc(cc))
	}

	// add Bcc recipients
	if bcc := header.Get("Bcc"); bcc != "" {
		logger = logger.With("bcc", bcc)
		opts = append(opts, sendmail.WithBcc(bcc))
	}

	// handle HTML or text bodies
	if msg.TextBody == "" {
		logger = logger.With("size", len(msg.HTMLBody), "type", "text/html")
		opts = append(opts, sendmail.WithBody(msg.HTMLBody), sendmail.WithHTMLContent())
	} else if msg.HTMLBody == "" {
		logger = logger.With("size", len(msg.TextBody), "type", "text/plain")
		opts = append(opts, sendmail.WithBody(msg.TextBody))
	}

	// create the request ready to POST
	message := sendmail.NewMessage(from, to, subject, opts...)

	// create user object
	user := client.Users().ByUserId(from)

	// send email
	if err := message.Send(context.Background(), user); err != nil {
		logger.Error("error sending email", "error", err)
		os.Exit(1)
	}

	logger.Info("message sent")
}
