// Package logger provides structured logging configuration with support for development and production environments.
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// Format types for logging.
	formatJSON   = "json"
	formatPretty = "pretty"
)

// ANSI color codes.
const (
	colorReset   = "\033[0m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
	colorGray    = "\033[37m"
	colorBold    = "\033[1m"
	colorDim     = "\033[2m"
)

// Logger wraps slog.Logger with additional functionality.
type Logger struct {
	*slog.Logger
}

// Config holds logger configuration.
type Config struct {
	Writer      io.Writer
	Format      string
	Environment string
	Level       slog.Level
	AddSource   bool
}

// New creates a new logger with the given configuration.
func New(cfg Config) *Logger {
	if cfg.Writer == nil {
		cfg.Writer = os.Stdout
	}

	// Auto-detect format based on environment if not specified.
	if cfg.Format == "" {
		if cfg.Environment == "production" {
			cfg.Format = formatJSON
		} else {
			cfg.Format = formatPretty
		}
	}

	var handler slog.Handler

	opts := &slog.HandlerOptions{
		Level:     cfg.Level,
		AddSource: cfg.AddSource,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			// Shorten source file paths.
			if a.Key == slog.SourceKey {
				if source, ok := a.Value.Any().(*slog.Source); ok {
					// Only show relative path from project root.
					source.File = filepath.Base(source.File)
				}
			}
			return a
		},
	}

	if cfg.Format == formatJSON {
		handler = slog.NewJSONHandler(cfg.Writer, opts)
	} else {
		// Use our custom pretty handler with colors.
		handler = NewPrettyHandler(cfg.Writer, opts)
	}

	return &Logger{
		Logger: slog.New(handler),
	}
}

// ParseLevel converts a string to slog.Level.
func ParseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// PrettyHandler is a custom slog.Handler that formats logs in a human-readable way with colors.
type PrettyHandler struct {
	opts   *slog.HandlerOptions
	writer io.Writer
	attrs  []slog.Attr
	groups []string
}

// NewPrettyHandler creates a new pretty handler.
func NewPrettyHandler(w io.Writer, opts *slog.HandlerOptions) *PrettyHandler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &PrettyHandler{
		opts:   opts,
		writer: w,
		attrs:  []slog.Attr{},
		groups: []string{},
	}
}

// Enabled reports whether the handler handles records at the given level.
func (h *PrettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	minLevel := slog.LevelInfo
	if h.opts.Level != nil {
		minLevel = h.opts.Level.Level()
	}
	return level >= minLevel
}

// Handle formats and writes the log record.
func (h *PrettyHandler) Handle(_ context.Context, r slog.Record) error {
	// Format: [TIME] LEVEL message key=value key=value.
	buf := make([]byte, 0, 1024)

	// Time with color.
	timeStr := r.Time.Format("15:04:05")
	buf = append(buf, colorDim...)
	buf = append(buf, timeStr...)
	buf = append(buf, colorReset...)
	buf = append(buf, ' ')

	// Level with color and icon.
	levelStr, levelColor := formatLevel(r.Level)
	buf = append(buf, levelColor...)
	buf = append(buf, levelStr...)
	buf = append(buf, colorReset...)
	buf = append(buf, ' ')

	// Source location if enabled.
	if h.opts.AddSource && r.PC != 0 {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		f, _ := fs.Next()
		buf = append(buf, colorDim...)
		buf = append(buf, filepath.Base(f.File)...)
		buf = append(buf, ':')
		buf = append(buf, strconv.Itoa(f.Line)...)
		buf = append(buf, colorReset...)
		buf = append(buf, ' ')
	}

	// Message with bold.
	buf = append(buf, colorBold...)
	buf = append(buf, r.Message...)
	buf = append(buf, colorReset...)

	// Attributes.
	attrs := make([]slog.Attr, 0, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})

	// Add pre-existing attributes from WithAttrs.
	attrs = append(h.attrs, attrs...)

	if len(attrs) > 0 {
		buf = append(buf, ' ')
		buf = append(buf, colorCyan...)
		for i, attr := range attrs {
			if i > 0 {
				buf = append(buf, ' ')
			}
			buf = append(buf, attr.Key...)
			buf = append(buf, '=')
			buf = append(buf, formatValue(attr.Value)...)
		}
		buf = append(buf, colorReset...)
	}

	buf = append(buf, '\n')
	_, err := h.writer.Write(buf)
	return err
}

// WithAttrs returns a new handler with additional attributes.
func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)

	return &PrettyHandler{
		opts:   h.opts,
		writer: h.writer,
		attrs:  newAttrs,
		groups: h.groups,
	}
}

// WithGroup returns a new handler with the given group.
func (h *PrettyHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name

	return &PrettyHandler{
		opts:   h.opts,
		writer: h.writer,
		attrs:  h.attrs,
		groups: newGroups,
	}
}

// formatLevel returns the formatted level string with color.
func formatLevel(level slog.Level) (levelStr, levelColor string) {
	switch level {
	case slog.LevelDebug:
		return "DBG", colorMagenta
	case slog.LevelInfo:
		return "INF", colorGreen
	case slog.LevelWarn:
		return "WRN", colorYellow
	case slog.LevelError:
		return "ERR", colorRed
	default:
		return level.String(), colorGray
	}
}

// formatValue formats a slog.Value for pretty printing.
func formatValue(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		return v.String()
	case slog.KindTime:
		return v.Time().Format(time.RFC3339)
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindAny, slog.KindBool, slog.KindFloat64, slog.KindInt64, slog.KindUint64, slog.KindGroup, slog.KindLogValuer:
		return v.String()
	default:
		return v.String()
	}
}

// Helper methods for common logging patterns.

// WithError adds an error attribute to the logger.
func (l *Logger) WithError(err error) *Logger {
	return &Logger{
		Logger: l.With(slog.String("error", err.Error())),
	}
}

// WithField adds a single field to the logger.
func (l *Logger) WithField(key string, value any) *Logger {
	return &Logger{
		Logger: l.With(slog.Any(key, value)),
	}
}

// WithFields adds multiple fields to the logger.
func (l *Logger) WithFields(fields map[string]any) *Logger {
	args := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}
	return &Logger{
		Logger: l.With(args...),
	}
}

// Fatal logs a fatal error and exits.
func (l *Logger) Fatal(msg string, args ...any) {
	l.Error(msg, args...)
	os.Exit(1)
}

// Fatalf logs a formatted fatal error and exits.
func (l *Logger) Fatalf(format string, args ...any) {
	l.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}
