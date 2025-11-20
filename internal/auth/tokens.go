package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json/v2"
	"fmt"
	"time"

	"aidanwoods.dev/go-paseto"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
)

const (
	tokenIssuer   = "listenup-server"
	tokenAudience = "listenup-client"

	// PASETO v4 symmetric key requirements.
	keyBytesSize     = 32 // 256 bits
	keyHexSize       = 64 // 32 bytes as hex string
	refreshTokenSize = 32 // 256 bits of entropy
)

// TokenService handles PASETO token generation and verification.
type TokenService struct {
	symmetricKey         paseto.V4SymmetricKey
	accessTokenDuration  time.Duration
	refreshTokenDuration time.Duration
}

// NewTokenService creates a new token service with the given configuration.
func NewTokenService(keyHex string, accessDuration, refreshDuration time.Duration) (*TokenService, error) {
	if len(keyHex) != keyHexSize {
		return nil, fmt.Errorf("PASETO v4 key must be exactly %d hex characters (%d bytes), got %d", keyHexSize, keyBytesSize, len(keyHex))
	}

	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid hex string for PASETO key: %w", err)
	}

	if len(keyBytes) != keyBytesSize {
		return nil, fmt.Errorf("decoded key must be exactly %d bytes, got %d", keyBytesSize, len(keyBytes))
	}

	key, err := paseto.V4SymmetricKeyFromBytes(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create PASETO symmetric key: %w", err)
	}

	return &TokenService{
		symmetricKey:         key,
		accessTokenDuration:  accessDuration,
		refreshTokenDuration: refreshDuration,
	}, nil
}

// GenerateAccessToken creates a new PASETO v4.local access token for the user
// The token is encrypted and contains user claims.
func (s *TokenService) GenerateAccessToken(user *domain.User) (string, error) {
	now := time.Now()

	token := paseto.NewToken()

	// Add the standard claims
	token.SetIssuer(tokenIssuer)
	token.SetSubject(user.ID)
	token.SetAudience(tokenAudience)
	token.SetIssuedAt(now)
	token.SetNotBefore(now)
	token.SetExpiration(now.Add(s.accessTokenDuration))

	// Generate unique token ID
	tokenID, err := id.Generate("token")
	if err != nil {
		return "", fmt.Errorf("generate token ID: %w", err)
	}
	token.SetJti(tokenID)

	// Our custom claims
	//nolint:errcheck // Token.Set only errors on invalid types, which we control
	_ = token.Set("user_id", user.ID)
	//nolint:errcheck // Token.Set only errors on invalid types, which we control
	_ = token.Set("email", user.Email)

	// Let's encrypt.
	encrypted := token.V4Encrypt(s.symmetricKey, nil)
	return encrypted, nil
}

// VerifyAccessToken verifies and parses a PASETO access tokne.
// Returns the claims if valid, or an error if they're invalid or expired.
func (s *TokenService) VerifyAccessToken(tokenString string) (*AccessClaims, error) {
	parser := paseto.NewParser()

	// Add validation rules (Basically just checks the claims we set above)
	parser.AddRule(paseto.ForAudience(tokenAudience))
	parser.AddRule(paseto.IssuedBy(tokenIssuer))
	parser.AddRule(paseto.NotExpired())
	parser.AddRule(paseto.ValidAt(time.Now()))

	// Parse and decrypt v4.local token
	token, err := parser.ParseV4Local(s.symmetricKey, tokenString, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	var claims AccessClaims
	if err := json.Unmarshal(token.ClaimsJSON(), &claims); err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}

	return &claims, nil
}

// GenerateRefreshToken creates a cryptographically random opaque refresh token.
// NOTE: this is NOT a PASETO token, it's just random bytes stored in the database that we can validate against
// Returns the token string in a base64-urlencoded format.
func (s *TokenService) GenerateRefreshToken() (string, error) {
	b := make([]byte, refreshTokenSize)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}

	return base64.URLEncoding.EncodeToString(b), nil
}

// HashRefreshToken creates a hash of the refresh token for database storage.
// We store hashed tokens so database compromise doesn't leak valid tokens (that would be bad).
// Uses hex encoding for simplicity (not trying to hide length, just prevent reuse).
func HashRefreshToken(token string) string {
	decoded, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return hex.EncodeToString([]byte(token))
	}
	return hex.EncodeToString(decoded)
}

// AccessTokenDuration returns the configured access token lifetime.
func (s *TokenService) AccessTokenDuration() time.Duration {
	return s.accessTokenDuration
}

// RefreshTokenDuration returns the configured refresh token lifetime.
func (s *TokenService) RefreshTokenDuration() time.Duration {
	return s.refreshTokenDuration
}
