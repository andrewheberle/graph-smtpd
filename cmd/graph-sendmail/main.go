package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/andrewheberle/configger"
	"github.com/andrewheberle/graph-smtpd/internal/cli"
	"github.com/andrewheberle/graph-smtpd/pkg/sendmail"
	"github.com/spf13/pflag"
)

var Version = "dev"

// exTempFail is sendmail's exit status (EX_TEMPFAIL from sysexits.h) for a
// failure that may succeed if retried.
const exTempFail = 75

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
	f.Duration("timeout", 30*time.Second, "Time allowed to send the message via Graph (0 for no limit)")

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
		if !quietWithoutConfig(f) {
			logger.Error("error loading configuration", "error", err)
		}
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
		logger.Error("could not create graph mailer", "error", err)
		os.Exit(1)
	}

	// build message from stdin, using headers in place of an envelope, and
	// count its size
	started := time.Now()
	stdin := &countingReader{r: os.Stdin}
	message, err := sendmail.FromMIME(stdin, sendmail.Envelope{}, sendmail.WithSaveToSentItems(k.Bool("sentitems")))
	_, _ = io.Copy(io.Discard, stdin)
	if err != nil {
		logResult(logger, nil, "rejected", stdin.n, started, err)
		os.Exit(1)
	}

	// send email, within the timeout if one is set
	ctx := context.Background()
	if timeout := k.Duration("timeout"); timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	if err := mailer.Send(ctx, message); err != nil {
		code := sendExitCode(err)
		status := "rejected"
		if code == exTempFail {
			status = "deferred"
		}
		logResult(logger, message, status, stdin.n, started, err)
		os.Exit(code)
	}

	logResult(logger, message, "sent", stdin.n, started, nil)
}

// logResult logs the outcome of sending a message as a single "message"
// line, with the same fields graph-smtpd uses: sent at INFO, otherwise at
// WARN with the reason. Header details are only logged for debugging.
func logResult(logger *slog.Logger, message *sendmail.Message, status string, size int64, started time.Time, reason error) {
	var attrs []any
	if message != nil {
		attrs = append(attrs, "from", message.From())
		if id := message.MessageID(); id != "" {
			attrs = append(attrs, "message_id", id)
		}
	}
	attrs = append(attrs, "size", size, "status", status)
	if reason != nil {
		attrs = append(attrs, "reason", reason.Error())
	}
	attrs = append(attrs, "duration", time.Since(started).Round(time.Millisecond).String())

	level := slog.LevelInfo
	if status != "sent" {
		level = slog.LevelWarn
	}
	logger.Log(context.Background(), level, "message", attrs...)

	if message != nil {
		logger.Debug("message details", "message", message)
	}
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

// loadConfig loads configuration from the config file, SENDMAIL_
// environment variables and the parsed command line flags in f.
func loadConfig(f *pflag.FlagSet) (configger.Config, error) {
	return configger.LoadConfig(f, configger.WithEnvPrefix("SENDMAIL"))
}

// sendExitCode returns the exit status for a failed send: 1 when Graph
// rejected the message, and exTempFail when a retry may succeed.
func sendExitCode(err error) int {
	if errors.Is(err, sendmail.ErrRejected) {
		return 1
	}

	return exTempFail
}

// quietWithoutConfig reports whether quiet mode was requested when the
// configuration could not be loaded, from the --quiet flag or the
// SENDMAIL_QUIET environment variable, the sources that do not depend on the
// configuration file.
func quietWithoutConfig(f *pflag.FlagSet) bool {
	if quiet, err := f.GetBool("quiet"); err == nil && quiet {
		return true
	}
	quiet, err := strconv.ParseBool(os.Getenv("SENDMAIL_QUIET"))

	return err == nil && quiet
}
