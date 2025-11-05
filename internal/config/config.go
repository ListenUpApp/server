package config

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	App      AppConfig
	Logger   LoggerConfig
	Metadata MetadataConfig
}

type AppConfig struct {
	Environment string
}

type LoggerConfig struct {
	Level string
}

type MetadataConfig struct {
	BasePath string
}

// LoadConfig loads configuration from multiple sources with precedence:
// 1. Command-line flags (highest priority)
// 2. Environment variables
// 3. .env file
// 4. Default values (lowest priority)
func LoadConfig() (*Config, error) {
	// Define command-line flags
	env := flag.String("env", "", "Environment (development, staging, production)")
	logLevel := flag.String("log-level", "", "Log level (debug, info, warn, error)")
	metadataPath := flag.String("metadata-path", "", "Base path for metadata storage")
	envFile := flag.String("env-file", ".env", "Path to .env file")

	// Parse flags but don't exit on error - we want to handle it gracefully
	flag.Parse()

	// Load .env file if it exists (silently ignore if not found)
	_ = loadEnvFile(*envFile)

	// Build config with proper precedence
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
	}

	// Expand and validate metadata path
	if err := cfg.expandMetadataPath(); err != nil {
		return nil, fmt.Errorf("invalid metadata path: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// Validate checks that all required config values are present and valid
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

	return nil
}

// expandMetadataPath expands ~ and makes the path absolute
func (c *Config) expandMetadataPath() error {
	path := c.Metadata.BasePath

	// If empty, use default
	if path == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		c.Metadata.BasePath = filepath.Join(homeDir, "ListenUp", "metadata")
		return nil
	}

	// Expand tilde
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(homeDir, path[2:])
	}

	// Make absolute if needed
	if !filepath.IsAbs(path) {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("failed to get absolute path: %w", err)
		}
		path = absPath
	}

	c.Metadata.BasePath = filepath.Clean(path)
	return nil
}

// getConfigValue returns the first non-empty value from flag, env var, or default
func getConfigValue(flagValue, envKey, defaultValue string) string {
	// Priority 1: Command-line flag
	if flagValue != "" {
		return flagValue
	}

	// Priority 2: Environment variable
	if envValue := os.Getenv(envKey); envValue != "" {
		return envValue
	}

	// Priority 3: Default value
	return defaultValue
}

// loadEnvFile loads environment variables from a .env file
// Format: KEY=value (one per line, # for comments)
func loadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid format at line %d: %s", lineNum, line)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		value = strings.Trim(value, `"'`)

		// Only set if not already set (env vars take precedence over .env file)
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("failed to set env var %s: %w", key, err)
			}
		}
	}

	return scanner.Err()
}
