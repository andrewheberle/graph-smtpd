package graphserver

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestErrorLog(t *testing.T) {
	var buf bytes.Buffer
	l := NewErrorLog(slog.New(slog.NewJSONHandler(&buf, nil)))

	l.Printf("accept error: %v", "connection reset")
	l.Println("handshake", "failed")

	var want = []string{"accept error: connection reset", "handshake failed"}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d: %s", len(lines), len(want), buf.String())
	}
	for i, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("log line is not JSON: %q", line)
		}
		if entry["level"] != "WARN" || entry["msg"] != "smtp server error" || entry["error"] != want[i] {
			t.Errorf("line %d = %v, want WARN smtp server error with error %q", i, entry, want[i])
		}
	}
}
