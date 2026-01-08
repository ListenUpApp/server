package providers

import (
	"encoding/hex"

	"github.com/samber/do/v2"

	"github.com/listenupapp/listenup-server/internal/auth"
	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/logger"
)

// AuthKey wraps the authentication key bytes.
type AuthKey []byte

// ProvideAuthKey loads or generates the authentication key.
func ProvideAuthKey(i do.Injector) (AuthKey, error) {
	cfg := do.MustInvoke[*config.Config](i)
	log := do.MustInvoke[*logger.Logger](i)

	key, err := auth.LoadOrGenerateKey(cfg.Metadata.BasePath)
	if err != nil {
		return nil, err
	}

	// Update config with the loaded key
	cfg.Auth.AccessTokenKey = key

	log.Info("Authentication key loaded",
		"access_token_duration", cfg.Auth.AccessTokenDuration,
		"refresh_token_duration", cfg.Auth.RefreshTokenDuration,
	)

	return AuthKey(key), nil
}

// ProvideTokenService provides the PASETO token service.
func ProvideTokenService(i do.Injector) (*auth.TokenService, error) {
	cfg := do.MustInvoke[*config.Config](i)
	authKey := do.MustInvoke[AuthKey](i)

	keyHex := hex.EncodeToString([]byte(authKey))
	return auth.NewTokenService(keyHex, cfg.Auth.AccessTokenDuration, cfg.Auth.RefreshTokenDuration)
}
