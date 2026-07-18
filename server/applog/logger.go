package applog

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Category string

const (
	CategorySystem     Category = "SYSTEM"
	CategoryUsage      Category = "USAGE"
	CategoryMiddleware Category = "MIDDLEWARE"
)

type Logger struct {
	inner *slog.Logger
}

func New() *Logger {
	return NewForWriter(os.Stdout)
}

func NewForWriter(w io.Writer) *Logger {
	return &Logger{
		inner: slog.New(newHumanHandler(w, slog.LevelInfo)),
	}
}

func (l *Logger) Info(category Category, msg string, attrs ...slog.Attr) {
	l.log(context.Background(), slog.LevelInfo, category, msg, attrs...)
}

func (l *Logger) Warn(category Category, msg string, attrs ...slog.Attr) {
	l.log(context.Background(), slog.LevelWarn, category, msg, attrs...)
}

func (l *Logger) Error(category Category, msg string, attrs ...slog.Attr) {
	l.log(context.Background(), slog.LevelError, category, msg, attrs...)
}

func (l *Logger) log(ctx context.Context, level slog.Level, category Category, msg string, attrs ...slog.Attr) {
	if l == nil {
		return
	}

	args := make([]any, 0, len(attrs)+1)
	args = append(args, slog.String("category", string(category)))
	for _, attr := range attrs {
		args = append(args, attr)
	}

	l.inner.Log(ctx, level, msg, args...)
}

type humanHandler struct {
	writer io.Writer
	level  slog.Leveler
	attrs  []slog.Attr
	groups []string
	mu     *sync.Mutex
}

func newHumanHandler(w io.Writer, level slog.Leveler) slog.Handler {
	return &humanHandler{
		writer: w,
		level:  level,
		mu:     &sync.Mutex{},
	}
}

func (h *humanHandler) Enabled(_ context.Context, level slog.Level) bool {
	minLevel := slog.LevelInfo
	if h.level != nil {
		minLevel = h.level.Level()
	}
	return level >= minLevel
}

func (h *humanHandler) Handle(_ context.Context, record slog.Record) error {
	fields := make(map[string]string)
	groupPrefix := strings.Join(h.groups, ".")

	for _, attr := range h.attrs {
		collectAttr(fields, groupPrefix, attr)
	}

	record.Attrs(func(attr slog.Attr) bool {
		collectAttr(fields, groupPrefix, attr)
		return true
	})

	category := fields["category"]
	delete(fields, "category")
	if category == "" {
		category = string(CategorySystem)
	}

	timestamp := record.Time
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	var builder strings.Builder
	builder.WriteString(timestamp.Format("02.01.2006, 15:04:05"))
	builder.WriteString(" [")
	builder.WriteString(category)
	builder.WriteString("] [")
	builder.WriteString(levelLabel(record.Level))
	builder.WriteString("] ")
	builder.WriteString(record.Message)

	for _, key := range orderedKeys(fields) {
		builder.WriteByte(' ')
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(formatFieldValue(fields[key]))
	}

	builder.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()

	_, err := io.WriteString(h.writer, builder.String())
	return err
}

func (h *humanHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	copied := *h
	copied.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &copied
}

func (h *humanHandler) WithGroup(name string) slog.Handler {
	copied := *h
	copied.groups = append(append([]string{}, h.groups...), name)
	return &copied
}

func collectAttr(fields map[string]string, prefix string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()

	if attr.Equal(slog.Attr{}) {
		return
	}

	key := attr.Key
	if prefix != "" && key != "" {
		key = prefix + "." + key
	}

	if attr.Value.Kind() == slog.KindGroup {
		nextPrefix := key
		for _, nested := range attr.Value.Group() {
			collectAttr(fields, nextPrefix, nested)
		}
		return
	}

	if key == "" {
		return
	}

	fields[key] = valueString(attr.Value)
}

func valueString(value slog.Value) string {
	switch value.Kind() {
	case slog.KindString:
		return value.String()
	case slog.KindInt64:
		return strconv.FormatInt(value.Int64(), 10)
	case slog.KindUint64:
		return strconv.FormatUint(value.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.FormatFloat(value.Float64(), 'f', -1, 64)
	case slog.KindBool:
		return strconv.FormatBool(value.Bool())
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindTime:
		return value.Time().Format(time.RFC3339)
	default:
		return fmt.Sprint(value.Any())
	}
}

func orderedKeys(fields map[string]string) []string {
	priority := []string{
		"request_id",
		"ip",
		"route",
		"method",
		"status",
		"entry_count",
		"duration_ms",
		"limit_per_min",
		"burst",
		"retry_after_s",
		"error",
	}

	keys := make([]string, 0, len(fields))
	seen := make(map[string]bool, len(fields))

	for _, key := range priority {
		if _, ok := fields[key]; ok {
			keys = append(keys, key)
			seen[key] = true
		}
	}

	extra := make([]string, 0, len(fields))
	for key := range fields {
		if !seen[key] {
			extra = append(extra, key)
		}
	}

	sort.Strings(extra)
	keys = append(keys, extra...)
	return keys
}

func formatFieldValue(value string) string {
	if value == "" {
		return `""`
	}

	if strings.ContainsAny(value, " \t\r\n=") {
		return strconv.Quote(value)
	}

	return value
}

func levelLabel(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "ERROR"
	case level >= slog.LevelWarn:
		return "WARNING"
	case level >= slog.LevelInfo:
		return "INFO"
	default:
		return "DEBUG"
	}
}
