package logger

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_DefaultWriter(t *testing.T) {
	cfg := Config{
		Level:  slog.LevelInfo,
		Format: "json",
	}

	logger := New(cfg)
	assert.NotNil(t, logger)
	assert.NotNil(t, logger.Logger)
}

func TestNew_CustomWriter(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Level:  slog.LevelInfo,
		Format: "json",
		Writer: &buf,
	}

	logger := New(cfg)
	logger.Info("test message")

	assert.Contains(t, buf.String(), "test message")
	assert.Contains(t, buf.String(), "\"level\":\"INFO\"")
}

func TestNew_FormatAutoDetection(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		wantFormat  string
	}{
		{
			name:        "production uses json",
			environment: "production",
			wantFormat:  "json",
		},
		{
			name:        "development uses pretty",
			environment: "development",
			wantFormat:  "pretty",
		},
		{
			name:        "staging uses pretty",
			environment: "staging",
			wantFormat:  "pretty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := Config{
				Level:       slog.LevelInfo,
				Environment: tt.environment,
				Writer:      &buf,
			}

			logger := New(cfg)
			logger.Info("test")

			output := buf.String()
			if tt.wantFormat == "json" {
				assert.Contains(t, output, `"msg":"test"`)
			} else {
				// Pretty format should contain ANSI codes
				assert.Contains(t, output, "test")
				// Should have some color codes (though exact format may vary)
				assert.True(t, len(output) > len("test\n"))
			}
		})
	}
}

func TestNew_ExplicitFormat(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Level:       slog.LevelInfo,
		Format:      "json",
		Environment: "development", // Would normally use pretty
		Writer:      &buf,
	}

	logger := New(cfg)
	logger.Info("test")

	// Should use JSON despite development environment
	assert.Contains(t, buf.String(), `"msg":"test"`)
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"DeBuG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"WARN", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"WARNING", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"unknown", slog.LevelInfo}, // defaults to info
		{"", slog.LevelInfo},        // defaults to info
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseLevel(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPrettyHandler_Enabled(t *testing.T) {
	tests := []struct {
		name         string
		handlerLevel slog.Level
		checkLevel   slog.Level
		wantEnabled  bool
	}{
		{
			name:         "debug handler allows debug",
			handlerLevel: slog.LevelDebug,
			checkLevel:   slog.LevelDebug,
			wantEnabled:  true,
		},
		{
			name:         "info handler blocks debug",
			handlerLevel: slog.LevelInfo,
			checkLevel:   slog.LevelDebug,
			wantEnabled:  false,
		},
		{
			name:         "info handler allows info",
			handlerLevel: slog.LevelInfo,
			checkLevel:   slog.LevelInfo,
			wantEnabled:  true,
		},
		{
			name:         "info handler allows error",
			handlerLevel: slog.LevelInfo,
			checkLevel:   slog.LevelError,
			wantEnabled:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			handler := NewPrettyHandler(&buf, &slog.HandlerOptions{
				Level: tt.handlerLevel,
			})

			enabled := handler.Enabled(context.Background(), tt.checkLevel)
			assert.Equal(t, tt.wantEnabled, enabled)
		})
	}
}

func TestPrettyHandler_Handle(t *testing.T) {
	var buf bytes.Buffer
	handler := NewPrettyHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	logger := slog.New(handler)
	logger.Info("test message", "key1", "value1", "key2", 42)

	output := buf.String()
	assert.Contains(t, output, "test message")
	assert.Contains(t, output, "key1=value1")
	assert.Contains(t, output, "key2=42")
	assert.Contains(t, output, "INF") // Level indicator
}

func TestPrettyHandler_LevelFormatting(t *testing.T) {
	tests := []struct {
		level      slog.Level
		wantString string
	}{
		{slog.LevelDebug, "DBG"},
		{slog.LevelInfo, "INF"},
		{slog.LevelWarn, "WRN"},
		{slog.LevelError, "ERR"},
	}

	for _, tt := range tests {
		t.Run(tt.wantString, func(t *testing.T) {
			var buf bytes.Buffer
			handler := NewPrettyHandler(&buf, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			})

			logger := slog.New(handler)
			logger.Log(context.Background(), tt.level, "test")

			assert.Contains(t, buf.String(), tt.wantString)
		})
	}
}

func TestPrettyHandler_WithAttrs(t *testing.T) {
	var buf bytes.Buffer
	handler := NewPrettyHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	// Add attributes to handler
	handlerWithAttrs := handler.WithAttrs([]slog.Attr{
		slog.String("service", "test-service"),
		slog.Int("version", 1),
	})

	logger := slog.New(handlerWithAttrs)
	logger.Info("test message")

	output := buf.String()
	assert.Contains(t, output, "service=test-service")
	assert.Contains(t, output, "version=1")
	assert.Contains(t, output, "test message")
}

func TestPrettyHandler_WithGroup(t *testing.T) {
	var buf bytes.Buffer
	handler := NewPrettyHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	// Add group (empty group should return same handler)
	handlerWithEmptyGroup := handler.WithGroup("")
	assert.Equal(t, handler, handlerWithEmptyGroup)

	// Add actual group
	handlerWithGroup := handler.WithGroup("request")
	assert.NotEqual(t, handler, handlerWithGroup)

	logger := slog.New(handlerWithGroup)
	logger.Info("test message")

	// Should still log the message
	assert.Contains(t, buf.String(), "test message")
}

func TestPrettyHandler_WithSource(t *testing.T) {
	var buf bytes.Buffer
	handler := NewPrettyHandler(&buf, &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: true,
	})

	logger := slog.New(handler)
	logger.Info("test message")

	output := buf.String()
	// Should contain source info (filename:line)
	assert.Contains(t, output, "logger_test.go:")
}

func TestFormatLevel(t *testing.T) {
	tests := []struct {
		level     slog.Level
		wantStr   string
		wantColor string
	}{
		{slog.LevelDebug, "DBG", colorMagenta},
		{slog.LevelInfo, "INF", colorGreen},
		{slog.LevelWarn, "WRN", colorYellow},
		{slog.LevelError, "ERR", colorRed},
	}

	for _, tt := range tests {
		t.Run(tt.wantStr, func(t *testing.T) {
			str, color := formatLevel(tt.level)
			assert.Equal(t, tt.wantStr, str)
			assert.Equal(t, tt.wantColor, color)
		})
	}
}

func TestFormatValue(t *testing.T) {
	now := time.Now()
	duration := 5 * time.Second

	tests := []struct {
		name  string
		value slog.Value
		want  string
	}{
		{
			name:  "string",
			value: slog.StringValue("test"),
			want:  "test",
		},
		{
			name:  "time",
			value: slog.TimeValue(now),
			want:  now.Format(time.RFC3339),
		},
		{
			name:  "duration",
			value: slog.DurationValue(duration),
			want:  "5s",
		},
		{
			name:  "int",
			value: slog.IntValue(42),
			want:  "42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatValue(tt.value)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLogger_WithError(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Level:  slog.LevelInfo,
		Format: "json",
		Writer: &buf,
	}

	logger := New(cfg)
	err := errors.New("test error")
	loggerWithErr := logger.WithError(err)

	loggerWithErr.Info("something happened")

	output := buf.String()
	assert.Contains(t, output, "test error")
	assert.Contains(t, output, "error")
}

func TestLogger_WithField(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Level:  slog.LevelInfo,
		Format: "json",
		Writer: &buf,
	}

	logger := New(cfg)
	loggerWithField := logger.WithField("user_id", "12345")

	loggerWithField.Info("user action")

	output := buf.String()
	assert.Contains(t, output, "user_id")
	assert.Contains(t, output, "12345")
	assert.Contains(t, output, "user action")
}

func TestLogger_WithFields(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Level:  slog.LevelInfo,
		Format: "json",
		Writer: &buf,
	}

	logger := New(cfg)
	loggerWithFields := logger.WithFields(map[string]any{
		"user_id":    "12345",
		"session_id": "abc-def",
		"ip":         "192.168.1.1",
	})

	loggerWithFields.Info("request processed")

	output := buf.String()
	assert.Contains(t, output, "user_id")
	assert.Contains(t, output, "12345")
	assert.Contains(t, output, "session_id")
	assert.Contains(t, output, "abc-def")
	assert.Contains(t, output, "ip")
	assert.Contains(t, output, "192.168.1.1")
}

func TestLogger_AllLevels(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Level:  slog.LevelDebug,
		Format: "pretty",
		Writer: &buf,
	}

	logger := New(cfg)

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	output := buf.String()
	assert.Contains(t, output, "debug message")
	assert.Contains(t, output, "info message")
	assert.Contains(t, output, "warn message")
	assert.Contains(t, output, "error message")

	// Check level indicators
	assert.Contains(t, output, "DBG")
	assert.Contains(t, output, "INF")
	assert.Contains(t, output, "WRN")
	assert.Contains(t, output, "ERR")
}

func TestLogger_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Level:  slog.LevelWarn, // Only warn and error
		Format: "json",
		Writer: &buf,
	}

	logger := New(cfg)

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	output := buf.String()
	// Should not contain debug or info
	assert.NotContains(t, output, "debug message")
	assert.NotContains(t, output, "info message")
	// Should contain warn and error
	assert.Contains(t, output, "warn message")
	assert.Contains(t, output, "error message")
}

func TestNewPrettyHandler_NilOptions(t *testing.T) {
	var buf bytes.Buffer
	handler := NewPrettyHandler(&buf, nil)

	assert.NotNil(t, handler)
	assert.NotNil(t, handler.opts)

	logger := slog.New(handler)
	logger.Info("test")

	assert.Contains(t, buf.String(), "test")
}

func TestPrettyHandler_MultipleAttributes(t *testing.T) {
	var buf bytes.Buffer
	handler := NewPrettyHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	logger := slog.New(handler)
	logger.Info("test",
		"string", "value",
		"int", 42,
		"bool", true,
		"float", 3.14,
	)

	output := buf.String()
	assert.Contains(t, output, "string=value")
	assert.Contains(t, output, "int=42")
	assert.Contains(t, output, "bool=true")
	assert.Contains(t, output, "float=3.14")
}

func TestPrettyHandler_TimeFormatting(t *testing.T) {
	var buf bytes.Buffer
	handler := NewPrettyHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	logger := slog.New(handler)
	logger.Info("test message")

	output := buf.String()
	// Should contain time in HH:MM:SS format
	timePattern := strings.Split(output, " ")[0]
	// Basic check that time format is there (e.g., "15:04:05")
	assert.True(t, len(timePattern) >= 8, "Should contain time prefix")
}

func TestLogger_ChainedWithMethods(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Level:  slog.LevelInfo,
		Format: "json",
		Writer: &buf,
	}

	logger := New(cfg)

	// Chain multiple With* methods
	logger.
		WithField("request_id", "req-123").
		WithError(errors.New("something failed")).
		WithFields(map[string]any{
			"user":   "john",
			"action": "login",
		}).
		Error("operation failed")

	output := buf.String()
	assert.Contains(t, output, "request_id")
	assert.Contains(t, output, "req-123")
	assert.Contains(t, output, "something failed")
	assert.Contains(t, output, "user")
	assert.Contains(t, output, "john")
	assert.Contains(t, output, "action")
	assert.Contains(t, output, "login")
	assert.Contains(t, output, "operation failed")
}

func TestPrettyHandler_EmptyMessage(t *testing.T) {
	var buf bytes.Buffer
	handler := NewPrettyHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	logger := slog.New(handler)
	logger.Info("")

	output := buf.String()
	// Should still produce output with time and level
	assert.Contains(t, output, "INF")
	assert.True(t, len(output) > 0)
}

func TestPrettyHandler_NoAttributes(t *testing.T) {
	var buf bytes.Buffer
	handler := NewPrettyHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	logger := slog.New(handler)
	logger.Info("simple message")

	output := buf.String()
	assert.Contains(t, output, "simple message")
	assert.Contains(t, output, "INF")
	// Should not have '=' characters indicating attributes
	parts := strings.Split(output, "simple message")
	if len(parts) > 1 {
		afterMessage := parts[1]
		// After message, should not have any attributes (no '=' signs)
		assert.NotContains(t, afterMessage, "=")
	}
}

func TestConfig_Defaults(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "minimal config",
			config: Config{
				Level: slog.LevelInfo,
			},
		},
		{
			name: "production config",
			config: Config{
				Level:       slog.LevelWarn,
				Environment: "production",
			},
		},
		{
			name: "development config",
			config: Config{
				Level:       slog.LevelDebug,
				Environment: "development",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := New(tt.config)
			require.NotNil(t, logger)
			require.NotNil(t, logger.Logger)
		})
	}
}
