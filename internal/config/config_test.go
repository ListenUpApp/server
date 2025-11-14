package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_ValidConfig(t *testing.T) {
	cfg := &Config{
		App: AppConfig{
			Environment: "development",
		},
		Logger: LoggerConfig{
			Level: "info",
		},
		Metadata: MetadataConfig{
			BasePath: "/some/path",
		},
		Library: LibraryConfig{
			AudiobookPath: "/audiobooks",
		},
	}

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidate_AllEnvironments(t *testing.T) {
	tests := []struct {
		env   string
		valid bool
	}{
		{"development", true},
		{"staging", true},
		{"production", true},
		{"test", false},
		{"", false},
		{"DEVELOPMENT", false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			cfg := &Config{
				App: AppConfig{
					Environment: tt.env,
				},
				Logger: LoggerConfig{
					Level: "info",
				},
				Metadata: MetadataConfig{
					BasePath: "/some/path",
				},
				Library: LibraryConfig{
					AudiobookPath: "/audiobooks",
				},
			}

			err := cfg.Validate()
			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestValidate_AllLogLevels(t *testing.T) {
	tests := []struct {
		level string
		valid bool
	}{
		{"debug", true},
		{"info", true},
		{"warn", true},
		{"error", true},
		{"DEBUG", true},  // case insensitive
		{"INFO", true},   // case insensitive
		{"trace", false}, // not supported
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			cfg := &Config{
				App: AppConfig{
					Environment: "development",
				},
				Logger: LoggerConfig{
					Level: tt.level,
				},
				Metadata: MetadataConfig{
					BasePath: "/some/path",
				},
				Library: LibraryConfig{
					AudiobookPath: "/audiobooks",
				},
			}

			err := cfg.Validate()
			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestValidate_EmptyMetadataPath(t *testing.T) {
	cfg := &Config{
		App: AppConfig{
			Environment: "development",
		},
		Logger: LoggerConfig{
			Level: "info",
		},
		Metadata: MetadataConfig{
			BasePath: "",
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "metadata base path cannot be empty")
}

func TestExpandMetadataPath_EmptyUsesDefault(t *testing.T) {
	cfg := &Config{
		Metadata: MetadataConfig{
			BasePath: "",
		},
	}

	err := cfg.expandMetadataPath()
	require.NoError(t, err)

	homeDir, _ := os.UserHomeDir() //nolint:errcheck // Test setup
	expected := filepath.Join(homeDir, "ListenUp", "metadata")
	assert.Equal(t, expected, cfg.Metadata.BasePath)
}

func TestExpandMetadataPath_TildeExpansion(t *testing.T) {
	cfg := &Config{
		Metadata: MetadataConfig{
			BasePath: "~/my-data",
		},
	}

	err := cfg.expandMetadataPath()
	require.NoError(t, err)

	homeDir, _ := os.UserHomeDir() //nolint:errcheck // Test setup
	expected := filepath.Join(homeDir, "my-data")
	assert.Equal(t, expected, cfg.Metadata.BasePath)
}

func TestExpandMetadataPath_AbsolutePath(t *testing.T) {
	cfg := &Config{
		Metadata: MetadataConfig{
			BasePath: "/absolute/path/to/data",
		},
	}

	err := cfg.expandMetadataPath()
	require.NoError(t, err)

	assert.Equal(t, "/absolute/path/to/data", cfg.Metadata.BasePath)
}

func TestExpandMetadataPath_RelativePath(t *testing.T) {
	cfg := &Config{
		Metadata: MetadataConfig{
			BasePath: "relative/path",
		},
	}

	err := cfg.expandMetadataPath()
	require.NoError(t, err)

	// Should be converted to absolute path.
	assert.True(t, filepath.IsAbs(cfg.Metadata.BasePath))
	assert.Contains(t, cfg.Metadata.BasePath, "relative/path")
}

func TestGetConfigValue_Precedence(t *testing.T) {
	// Test flag value takes priority.
	result := getConfigValue("flag-value", "ENV_KEY", "default-value")
	assert.Equal(t, "flag-value", result)

	// Test env var when flag is empty.
	os.Setenv("TEST_ENV_KEY", "env-value") //nolint:errcheck // Test setup
	defer os.Unsetenv("TEST_ENV_KEY")      //nolint:errcheck // Test cleanup

	result = getConfigValue("", "TEST_ENV_KEY", "default-value")
	assert.Equal(t, "env-value", result)

	// Test default when both are empty.
	result = getConfigValue("", "NONEXISTENT_KEY", "default-value")
	assert.Equal(t, "default-value", result)
}

func TestLoadEnvFile_ValidFile(t *testing.T) {
	// Create temp .env file.
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")

	content := `# Test env file
ENV=staging
LOG_LEVEL=debug
METADATA_PATH=/test/path
# Comment line
QUOTED_VALUE="some value"
SINGLE_QUOTED='another value'
`
	err := os.WriteFile(envFile, []byte(content), 0o644)
	require.NoError(t, err)

	// Clear any existing env vars.
	os.Unsetenv("ENV")           //nolint:errcheck // Test cleanup
	os.Unsetenv("LOG_LEVEL")     //nolint:errcheck // Test cleanup
	os.Unsetenv("METADATA_PATH") //nolint:errcheck // Test cleanup
	os.Unsetenv("QUOTED_VALUE")  //nolint:errcheck // Test cleanup
	os.Unsetenv("SINGLE_QUOTED") //nolint:errcheck // Test cleanup
	defer func() {
		os.Unsetenv("ENV")           //nolint:errcheck // Test cleanup
		os.Unsetenv("LOG_LEVEL")     //nolint:errcheck // Test cleanup
		os.Unsetenv("METADATA_PATH") //nolint:errcheck // Test cleanup
		os.Unsetenv("QUOTED_VALUE")  //nolint:errcheck // Test cleanup
		os.Unsetenv("SINGLE_QUOTED") //nolint:errcheck // Test cleanup
	}()

	// Load the file.
	err = loadEnvFile(envFile)
	require.NoError(t, err)

	// Verify values were loaded.
	assert.Equal(t, "staging", os.Getenv("ENV"))
	assert.Equal(t, "debug", os.Getenv("LOG_LEVEL"))
	assert.Equal(t, "/test/path", os.Getenv("METADATA_PATH"))
	assert.Equal(t, "some value", os.Getenv("QUOTED_VALUE"))
	assert.Equal(t, "another value", os.Getenv("SINGLE_QUOTED"))
}

func TestLoadEnvFile_InvalidFormat(t *testing.T) {
	// Create temp .env file with invalid format.
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")

	content := `VALID_KEY=valid_value
INVALID LINE WITHOUT EQUALS
ANOTHER_VALID=value
`
	err := os.WriteFile(envFile, []byte(content), 0o644)
	require.NoError(t, err)

	// Should return error.
	err = loadEnvFile(envFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid format")
}

func TestLoadEnvFile_NonExistentFile(t *testing.T) {
	err := loadEnvFile("/nonexistent/file/.env")
	assert.Error(t, err)
}

func TestLoadEnvFile_ExistingEnvVarsNotOverwritten(t *testing.T) {
	// Set env var first.
	os.Setenv("TEST_VAR", "original-value") //nolint:errcheck // Test setup
	defer os.Unsetenv("TEST_VAR")           //nolint:errcheck // Test cleanup

	// Create temp .env file that tries to override it.
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")

	content := `TEST_VAR=new-value`
	err := os.WriteFile(envFile, []byte(content), 0o644)
	require.NoError(t, err)

	// Load the file.
	err = loadEnvFile(envFile)
	require.NoError(t, err)

	// Original value should be preserved.
	assert.Equal(t, "original-value", os.Getenv("TEST_VAR"))
}

func TestLoadEnvFile_EmptyLines(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")

	content := `
KEY1=value1


KEY2=value2

# Comment

KEY3=value3
`
	err := os.WriteFile(envFile, []byte(content), 0o644)
	require.NoError(t, err)

	os.Unsetenv("KEY1") //nolint:errcheck // Test cleanup
	os.Unsetenv("KEY2") //nolint:errcheck // Test cleanup
	os.Unsetenv("KEY3") //nolint:errcheck // Test cleanup
	defer func() {
		os.Unsetenv("KEY1") //nolint:errcheck // Test cleanup
		os.Unsetenv("KEY2") //nolint:errcheck // Test cleanup
		os.Unsetenv("KEY3") //nolint:errcheck // Test cleanup
	}()

	err = loadEnvFile(envFile)
	require.NoError(t, err)

	assert.Equal(t, "value1", os.Getenv("KEY1"))
	assert.Equal(t, "value2", os.Getenv("KEY2"))
	assert.Equal(t, "value3", os.Getenv("KEY3"))
}

func TestLoadEnvFile_Whitespace(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")

	content := `  KEY_WITH_SPACES  =  value with spaces  `
	err := os.WriteFile(envFile, []byte(content), 0o644)
	require.NoError(t, err)

	os.Unsetenv("KEY_WITH_SPACES")       //nolint:errcheck // Test cleanup
	defer os.Unsetenv("KEY_WITH_SPACES") //nolint:errcheck // Test cleanup

	err = loadEnvFile(envFile)
	require.NoError(t, err)

	// Whitespace should be trimmed.
	assert.Equal(t, "value with spaces", os.Getenv("KEY_WITH_SPACES"))
}
