package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	// We're not trying to protect Fort Knox, just using some sensible defaults for a self-hosted app likely being run on a local network.
	// Maybe we make these configurable down the line, but I'm not sure if there's much point in that.
	argon2Memory      = 64 * 1024
	argon2Iterations  = 3
	argon2Parallelism = 4
	argon2SaltLength  = 16
	argon2KeyLength   = 32

	// Prevent DoS attacks from massive passwords consuming CPU/memory during hashing.
	// This is generous enough for any legitimate use case but stops casual abuse.
	// We're not protecting from state-backed hackers, but script kiddies looking for quick targets to pick off.
	maxPasswordLength = 1024
)

// HashPassword creates an Argon2 ID hash of the password.
// It returns a formatted string, or an error.
func HashPassword(password string) (string, error) {
	// Validate password to prevent DoS and catch bugs.
	if password == "" {
		return "", errors.New("password cannot be empty")
	}
	if len(password) > maxPasswordLength {
		return "", errors.New("password exceeds maximum length")
	}

	// Generate a Cryptographically secure salt.
	salt := make([]byte, argon2SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		argon2Iterations,
		argon2Memory,
		argon2Parallelism,
		argon2KeyLength,
	)

	// Base 64 Encode
	saltB64 := base64.RawStdEncoding.EncodeToString(salt)
	hashB64 := base64.RawStdEncoding.EncodeToString(hash)

	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argon2Memory,
		argon2Iterations,
		argon2Parallelism,
		saltB64,
		hashB64,
	)

	return encoded, nil
}

// VerifyPassword verifies a password against an Argon2id encoded hash.
func VerifyPassword(encodedHash, password string) (bool, error) {
	// Validate password length before doing expensive hashing.
	if len(password) > maxPasswordLength {
		return false, nil
	}

	salt, hash, params, err := decodeHash(encodedHash)
	if err != nil {
		// Just returning false to prevent leaking of sensitive information.
		//nolint:nilerr // Intentionally returning nil to avoid leaking hash validation details
		return false, nil
	}

	// Generate hash with the same parameters
	testHash := argon2.IDKey(
		[]byte(password),
		salt,
		params.iterations,
		params.memory,
		params.parallelism,
		params.keyLength,
	)

	// Constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare(hash, testHash) == 1 {
		return true, nil
	}

	return false, nil
}

// argon2Params holds the parameters extracted from an encoded hash.
type argon2Params struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
	keyLength   uint32
}

// decodeHash extracts salt, hash and parameters from encoded string, or errors if it can't.
func decodeHash(encodedHash string) (salt, hash []byte, params *argon2Params, err error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return nil, nil, nil, errors.New("invalid hash format")
	}

	// Verify algorithm is correct.
	if parts[1] != "argon2id" {
		return nil, nil, nil, fmt.Errorf("unsupported algorithm: %s", parts[1])
	}
	// Parse version
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return nil, nil, nil, fmt.Errorf("invalid version: %w", err)
	}
	if version != argon2.Version {
		return nil, nil, nil, fmt.Errorf("incompatible version: %d", version)
	}

	// Parse parameters
	params = &argon2Params{}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.memory, &params.iterations, &params.parallelism); err != nil {
		return nil, nil, nil, fmt.Errorf("invalid parameters: %w", err)
	}

	// Decode salt
	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid salt encoding: %w", err)
	}

	// Decode hash
	hash, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid hash encoding: %w", err)
	}

	//nolint:gosec // Hash length is always 32 bytes (argon2KeyLength), safe to convert
	params.keyLength = uint32(len(hash))

	return salt, hash, params, nil
}
