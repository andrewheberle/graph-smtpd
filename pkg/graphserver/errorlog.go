package graphserver

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/emersion/go-smtp"
)

// errorLog passes go-smtp's internal errors, such as failed TLS handshakes
// or connection errors, to an slog.Logger at WARN.
type errorLog struct {
	logger *slog.Logger
}

// NewErrorLog returns an smtp.Logger, for smtp.Server.ErrorLog, that logs
// to logger.
func NewErrorLog(logger *slog.Logger) smtp.Logger {
	return errorLog{logger: logger}
}

func (l errorLog) Printf(format string, v ...interface{}) {
	l.logger.Warn("smtp server error", "error", strings.TrimSpace(fmt.Sprintf(format, v...)))
}

func (l errorLog) Println(v ...interface{}) {
	l.logger.Warn("smtp server error", "error", strings.TrimSpace(fmt.Sprintln(v...)))
}
