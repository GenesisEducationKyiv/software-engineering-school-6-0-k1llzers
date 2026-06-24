//go:build unit

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad_ReturnsErrorWhenFileDoesNotExist(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	require.Error(t, err)
}

func TestLoad_ReadsConfigFromYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	err := os.WriteFile(path, []byte(`
server:
  port: "9090"
database:
  url: "postgres://user:pass@localhost:5432/custom?sslmode=disable"
github:
  token: "secret"
mail:
  host: "smtp.example.com"
  port: 2525
  username: "mailer"
  password: "pass"
  from: "noreply@example.com"
  api_base_url: "http://localhost:8080/api"
quota:
  grpc_port: "9091"
  default_subscription_limit: 5
logging:
  level: "debug"
  format: "text"
`), 0o644)
	require.NoError(t, err)

	cfg, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, "9090", cfg.Server.Port)
	require.Equal(t, "postgres://user:pass@localhost:5432/custom?sslmode=disable", cfg.Database.URL)
	require.Equal(t, "secret", cfg.GitHub.Token)
	require.Equal(t, "smtp.example.com", cfg.Mail.Host)
	require.Equal(t, 2525, cfg.Mail.Port)
	require.Equal(t, "mailer", cfg.Mail.Username)
	require.Equal(t, "pass", cfg.Mail.Password)
	require.Equal(t, "noreply@example.com", cfg.Mail.From)
	require.Equal(t, "http://localhost:8080/api", cfg.Mail.ApiBaseUrl)
	require.Equal(t, "9091", cfg.Quota.GRPCPort)
	require.Equal(t, 5, cfg.Quota.DefaultSubscriptionLimit)
	require.Equal(t, "debug", cfg.Logging.Level)
	require.Equal(t, "text", cfg.Logging.Format)
}

func TestLoad_DoesNotApplyDefaultsForMissingFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	err := os.WriteFile(path, []byte(`
github:
  token: "secret"
`), 0o644)
	require.NoError(t, err)

	cfg, err := Load(path)
	require.NoError(t, err)
	require.Empty(t, cfg.Server.Port)
	require.Empty(t, cfg.Database.URL)
	require.Equal(t, "secret", cfg.GitHub.Token)
	require.Zero(t, cfg.Mail.Port)
	require.Empty(t, cfg.Mail.ApiBaseUrl)
	require.Empty(t, cfg.Logging.Level)
	require.Empty(t, cfg.Logging.Format)
}

func TestLoad_ReturnsErrorForInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	err := os.WriteFile(path, []byte("server: ["), 0o644)
	require.NoError(t, err)

	_, err = Load(path)
	require.Error(t, err)
}

func TestLoad_ReadsPartialConfigWithoutFillingMissingValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	err := os.WriteFile(path, []byte(`
mail:
  host: "smtp.example.com"
`), 0o644)
	require.NoError(t, err)

	cfg, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, "smtp.example.com", cfg.Mail.Host)
	require.Zero(t, cfg.Mail.Port)
	require.Empty(t, cfg.Mail.ApiBaseUrl)
	require.Empty(t, cfg.Logging.Level)
	require.Empty(t, cfg.Logging.Format)
}

func TestLoad_ReturnsErrorWhenPathIsEmpty(t *testing.T) {
	_, err := Load("")
	require.Error(t, err)
}
