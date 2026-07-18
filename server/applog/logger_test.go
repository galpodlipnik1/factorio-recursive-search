package applog

import (
	"bytes"
	"log/slog"
	"regexp"
	"strings"
	"testing"
)

func TestLoggerFormatsReadableLine(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewForWriter(&buffer)

	logger.Warn(
		CategoryMiddleware,
		"rate limit exceeded",
		slog.String("ip", "203.0.113.10"),
		slog.String("route", "/index"),
		slog.Int("status", 429),
		slog.Int("limit_per_min", 5),
		slog.Int("burst", 2),
	)

	line := strings.TrimSpace(buffer.String())
	pattern := regexp.MustCompile(`^\d{2}\.\d{2}\.\d{4}, \d{2}:\d{2}:\d{2} \[MIDDLEWARE\] \[WARNING\] rate limit exceeded`)

	if !pattern.MatchString(line) {
		t.Fatalf("unexpected log format: %s", line)
	}

	for _, expected := range []string{
		`ip=203.0.113.10`,
		`route=/index`,
		`status=429`,
		`limit_per_min=5`,
		`burst=2`,
	} {
		if !strings.Contains(line, expected) {
			t.Fatalf("expected log line to contain %q, got %s", expected, line)
		}
	}
}
