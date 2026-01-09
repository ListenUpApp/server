// Package config provides application configuration management with support for environment variables, command-line flags, and .env files.
package config

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config holds the application configuration.
type Config struct {
	App       AppConfig
	Logger    LoggerConfig
	Metadata  MetadataConfig
	Library   LibraryConfig
	Server    ServerConfig
	Auth      AuthConfig
	Transcode TranscodeConfig
	Audible   AudibleConfig
}

// AppConfig holds application-level configuration.
type AppConfig struct {
	Environment string
}

// LoggerConfig holds logging configuration.
type LoggerConfig struct {
	Level string
}

// MetadataConfig holds metadata storage configuration.
type MetadataConfig struct {
	BasePath string
}

// LibraryConfig holds audiobook library configuration.
type LibraryConfig struct {
	// Adding an Env variable here so we can properly test E2E without adding in the concept of libraries yet.
	AudiobookPath string
}

// ServerConfig holds server configuration.
type ServerConfig struct {
	Name          string
	LocalURL      string        // Optional
	RemoteURL     string        // Optional
	Port          string        // Server port (default: 8080)
	ReadTimeout   time.Duration // HTTP read timeout (default: 15s)
	WriteTimeout  time.Duration // HTTP write timeout (default: 15s)
	IdleTimeout   time.Duration // HTTP idle timeout (default: 60s)
	AdvertiseMDNS bool          // Advertise via mDNS/Zeroconf (default: true)
}

// AuthConfig holds authentication configuration.
type AuthConfig struct {
	// PASETO v4 symmetric key for access tokens (32 bytes)
	AccessTokenKey []byte
	// Session durations
	AccessTokenDuration  time.Duration // e.g., 15m
	RefreshTokenDuration time.Duration // e.g., 720h (30 days)
}

// TranscodeConfig holds audio transcoding configuration.
type TranscodeConfig struct {
	// Enabled allows disabling transcoding entirely (default: true)
	Enabled bool
	// CachePath is the directory for cached transcodes (default: {metadata}/cache/transcode)
	CachePath string
	// MaxConcurrent is the maximum simultaneous transcode jobs (default: 2)
	MaxConcurrent int
	// FFmpegPath overrides auto-detection of ffmpeg location (default: auto-detect)
	FFmpegPath string
}

// AudibleConfig holds Audible API configuration.
type AudibleConfig struct {
	// DefaultRegion is the default Audible marketplace (default: us)
	// Valid values: us, uk, de, fr, au, ca, jp, it, in, es
	DefaultRegion string
}

// LoadConfig loads configuration from multiple sources with precedence:
// 1. Command-line flags (highest priority).
// 2. Environment variables.
// 3. .env file.
// 4. Default values (lowest priority).
func LoadConfig() (*Config, error) {
	// Define command-line flags.
	env := flag.String("env", "", "Environment (development, staging, production)")
	logLevel := flag.String("log-level", "", "Log level (debug, info, warn, error)")
	metadataPath := flag.String("metadata-path", "", "Base path for metadata storage")
	audiobookPath := flag.String("audiobook-path", "", "Path to audiobook library")
	serverName := flag.String("server-name", "", "Name for the server")
	serverLocalURL := flag.String("local-url", "", "Internal server url")
	serverRemoteURL := flag.String("remote-url", "", "Remote server url")

	// Auth flags
	accessTokenDuration := flag.String("access-token-duration", "", "Access token lifetime (e.g., 15m)")
	refreshTokenDuration := flag.String("refresh-token-duration", "", "Refresh token lifetime (e.g., 720h)")

	// Server flags
	serverPort := flag.String("port", "", "Server port (default: 8080)")
	readTimeout := flag.String("read-timeout", "", "HTTP read timeout (default: 15s)")
	writeTimeout := flag.String("write-timeout", "", "HTTP write timeout (default: 15s)")
	idleTimeout := flag.String("idle-timeout", "", "HTTP idle timeout (default: 60s)")
	advertiseMDNS := flag.String("advertise-mdns", "", "Advertise via mDNS/Zeroconf (default: true)")

	envFile := flag.String("env-file", ".env", "Path to .env file")

	// Transcode flags
	transcodeEnabled := flag.String("transcode-enabled", "", "Enable audio transcoding (default: true)")
	transcodeCachePath := flag.String("transcode-cache-path", "", "Path for transcode cache")
	transcodeMaxConcurrent := flag.String("transcode-max-concurrent", "", "Max concurrent transcode jobs (default: 2)")
	transcodeFFmpegPath := flag.String("ffmpeg-path", "", "Path to ffmpeg binary (default: auto-detect)")

	// Parse flags but don't exit on error - we want to handle it gracefully.
	flag.Parse()

	// Load .env file if it exists (silently ignore if not found).
	_ = loadEnvFile(*envFile)

	// Build config with proper precedence.
	cfg := &Config{
		App: AppConfig{
			Environment: getConfigValue(*env, "ENV", "development"),
		},
		Logger: LoggerConfig{
			Level: getConfigValue(*logLevel, "LOG_LEVEL", "info"),
		},
		Metadata: MetadataConfig{
			BasePath: getConfigValue(*metadataPath, "METADATA_PATH", ""),
		},

		Library: LibraryConfig{
			AudiobookPath: getConfigValue(*audiobookPath, "AUDIOBOOK_PATH", ""),
		},

		Server: ServerConfig{
			Name:          getConfigValue(*serverName, "SERVER_NAME", "ListenUp Server"),
			LocalURL:      getConfigValue(*serverLocalURL, "SERVER_LOCAL_URL", ""),
			RemoteURL:     getConfigValue(*serverRemoteURL, "SERVER_REMOTE_URL", ""),
			Port:          getConfigValue(*serverPort, "SERVER_PORT", "8080"),
			AdvertiseMDNS: getBoolConfigValue(*advertiseMDNS, "ADVERTISE_MDNS", true),
		},

		Auth: AuthConfig{
			AccessTokenKey: nil, // Will be set by auth.LoadOrGenerateKey in main
		},

		Transcode: TranscodeConfig{
			Enabled:       getBoolConfigValue(*transcodeEnabled, "TRANSCODE_ENABLED", true),
			CachePath:     getConfigValue(*transcodeCachePath, "TRANSCODE_CACHE_PATH", ""),
			MaxConcurrent: getIntConfigValue(*transcodeMaxConcurrent, "TRANSCODE_MAX_CONCURRENT", 2),
			FFmpegPath:    getConfigValue(*transcodeFFmpegPath, "FFMPEG_PATH", ""),
		},

		Audible: AudibleConfig{
			DefaultRegion: getConfigValue("", "AUDIBLE_DEFAULT_REGION", "us"),
		},
	}

	// Parse auth durations.
	accessDurationStr := getConfigValue(*accessTokenDuration, "ACCESS_TOKEN_DURATION", "15m")
	accessDuration, err := time.ParseDuration(accessDurationStr)
	if err != nil {
		return nil, fmt.Errorf("invalid access token duration %q: %w", accessDurationStr, err)
	}
	cfg.Auth.AccessTokenDuration = accessDuration

	refreshDurationStr := getConfigValue(*refreshTokenDuration, "REFRESH_TOKEN_DURATION", "720h")
	refreshDuration, err := time.ParseDuration(refreshDurationStr)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token duration %q: %w", refreshDurationStr, err)
	}
	cfg.Auth.RefreshTokenDuration = refreshDuration

	// Parse server timeouts.
	readTimeoutStr := getConfigValue(*readTimeout, "SERVER_READ_TIMEOUT", "15s")
	readTimeoutDuration, err := time.ParseDuration(readTimeoutStr)
	if err != nil {
		return nil, fmt.Errorf("invalid read timeout %q: %w", readTimeoutStr, err)
	}
	cfg.Server.ReadTimeout = readTimeoutDuration

	writeTimeoutStr := getConfigValue(*writeTimeout, "SERVER_WRITE_TIMEOUT", "15s")
	writeTimeoutDuration, err := time.ParseDuration(writeTimeoutStr)
	if err != nil {
		return nil, fmt.Errorf("invalid write timeout %q: %w", writeTimeoutStr, err)
	}
	cfg.Server.WriteTimeout = writeTimeoutDuration

	idleTimeoutStr := getConfigValue(*idleTimeout, "SERVER_IDLE_TIMEOUT", "60s")
	idleTimeoutDuration, err := time.ParseDuration(idleTimeoutStr)
	if err != nil {
		return nil, fmt.Errorf("invalid idle timeout %q: %w", idleTimeoutStr, err)
	}
	cfg.Server.IdleTimeout = idleTimeoutDuration

	// Expand and validate metadata path.
	if err := cfg.expandMetadataPath(); err != nil {
		return nil, fmt.Errorf("invalid metadata path: %w", err)
	}

	// Expand and validate audiobook path.
	if err := cfg.expandAudiobookPath(); err != nil {
		return nil, fmt.Errorf("invalid audiobook path: %w", err)
	}

	// Expand transcode cache path (defaults to {metadata}/cache/transcode).
	if err := cfg.expandTranscodeCachePath(); err != nil {
		return nil, fmt.Errorf("invalid transcode cache path: %w", err)
	}

	// Validate configuration.
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// Validate checks that all required config values are present and valid.
func (c *Config) Validate() error {
	if c.App.Environment == "" {
		return errors.New("ENV is required")
	}

	validEnvs := map[string]bool{
		"development": true,
		"staging":     true,
		"production":  true,
	}
	if !validEnvs[c.App.Environment] {
		return fmt.Errorf("invalid environment: %s (must be development, staging, or production)", c.App.Environment)
	}

	validLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLevels[strings.ToLower(c.Logger.Level)] {
		return fmt.Errorf("invalid log level: %s (must be debug, info, warn, or error)", c.Logger.Level)
	}

	if c.Metadata.BasePath == "" {
		return errors.New("metadata base path cannot be empty after expansion")
	}

	// AudiobookPath can be empty - library can be configured via API setup flow

	// Auth durations are validated during LoadConfig parsing.
	// Auth key is set by auth.LoadOrGenerateKey in main.

	return nil
}

// expandPath expands ~ and makes the path absolute.
// If path is empty and defaultPath is provided, uses the default.
func expandPath(path, defaultPath string) (string, error) {
	if path == "" {
		return defaultPath, nil
	}

	// Expand tilde.
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(homeDir, path[2:])
	}

	// Make absolute if needed.
	if !filepath.IsAbs(path) {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("failed to get absolute path: %w", err)
		}
		path = absPath
	}

	return filepath.Clean(path), nil
}

// expandMetadataPath expands ~ and makes the path absolute.
func (c *Config) expandMetadataPath() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	defaultPath := filepath.Join(homeDir, "ListenUp", "metadata")

	expanded, err := expandPath(c.Metadata.BasePath, defaultPath)
	if err != nil {
		return err
	}
	c.Metadata.BasePath = expanded
	return nil
}

// expandAudiobookPath expands ~ and makes the path absolute.
// If empty, leaves it empty to allow setup via API.
func (c *Config) expandAudiobookPath() error {
	// Allow empty path - library can be configured via API
	if c.Library.AudiobookPath == "" {
		return nil
	}

	expanded, err := expandPath(c.Library.AudiobookPath, "")
	if err != nil {
		return err
	}
	c.Library.AudiobookPath = expanded
	return nil
}

// expandTranscodeCachePath expands ~ and makes the path absolute.
// Defaults to {metadata}/cache/transcode if not specified.
func (c *Config) expandTranscodeCachePath() error {
	defaultPath := filepath.Join(c.Metadata.BasePath, "cache", "transcode")

	expanded, err := expandPath(c.Transcode.CachePath, defaultPath)
	if err != nil {
		return err
	}
	c.Transcode.CachePath = expanded
	return nil
}

// getConfigValue returns the first non-empty value from flag, env var, or default.
func getConfigValue(flagValue, envKey, defaultValue string) string {
	// Priority 1: Command-line flag.
	if flagValue != "" {
		return flagValue
	}

	// Priority 2: Environment variable.
	if envValue := os.Getenv(envKey); envValue != "" {
		return envValue
	}

	// Priority 3: Default value.
	return defaultValue
}

// getBoolConfigValue returns a bool from flag, env var, or default.
// Accepts: "true", "1", "yes" (case-insensitive) as true; anything else is false.
func getBoolConfigValue(flagValue, envKey string, defaultValue bool) bool {
	strValue := getConfigValue(flagValue, envKey, "")
	if strValue == "" {
		return defaultValue
	}
	strValue = strings.ToLower(strValue)
	return strValue == "true" || strValue == "1" || strValue == "yes"
}

// getIntConfigValue returns an int from flag, env var, or default.
func getIntConfigValue(flagValue, envKey string, defaultValue int) int {
	strValue := getConfigValue(flagValue, envKey, "")
	if strValue == "" {
		return defaultValue
	}
	var result int
	if _, err := fmt.Sscanf(strValue, "%d", &result); err != nil {
		return defaultValue
	}
	return result
}

// loadEnvFile loads environment variables from a .env file.
// Format: KEY=value (one per line, # for comments).
func loadEnvFile(path string) error {
	file, err := os.Open(path) //#nosec G304 -- Config file path from user input is expected
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=value.
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid format at line %d: %s", lineNum, line)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present.
		value = strings.Trim(value, `"'`)

		// Only set if not already set (env vars take precedence over .env file).
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("failed to set env var %s: %w", key, err)
			}
		}
	}

	return scanner.Err()
}
