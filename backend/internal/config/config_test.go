package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigValidation(t *testing.T) {
	valid := Config{
		AppEnv:             "development",
		AppPort:            "8080",
		DatabaseURL:        "postgres://cloudpulse:devpass@localhost:5432/cloudpulse",
		JWTSecret:          "this-is-a-secure-32-character-minimum-secret-key",
		JWTAccessTTL:       15 * time.Minute,
		JWTRefreshTTL:      168 * time.Hour,
		CORSAllowedOrigins: []string{"http://localhost:5173"},
	}
	require.NoError(t, valid.Validate())

	t.Run("rejects short jwt secret", func(t *testing.T) {
		c := valid
		c.JWTSecret = "too-short"
		err := c.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "32 characters")
	})

	t.Run("rejects unsafe production defaults", func(t *testing.T) {
		c := valid
		c.AppEnv = "production"
		c.JWTSecret = "change-me-use-a-long-random-32-plus-char-secret-key"
		c.DatabaseURL = "postgres://cloudpulse:change-me@postgres:5432/cloudpulse"
		c.CookieSecure = false
		c.CORSAllowedOrigins = []string{"*"}

		err := c.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "JWT_SECRET")
		assert.Contains(t, err.Error(), "DATABASE_URL")
		assert.Contains(t, err.Error(), "COOKIE_SECURE")
		assert.Contains(t, err.Error(), "wildcard")
	})
}
