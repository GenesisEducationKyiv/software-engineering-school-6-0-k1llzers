//go:build unit

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad_ReturnsDefaultsWhenFileDoesNotExist(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	require.NoError(t, err)
	require.Equal(t, Default(), cfg)
}

func TestLoad_OverridesDefaultsFromYAML(t *testing.T) {
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
	require.Equal(t, "debug", cfg.Logging.Level)
	require.Equal(t, "text", cfg.Logging.Format)
}

func TestLoad_AppliesDefaultsForMissingFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	err := os.WriteFile(path, []byte(`
github:
  token: "secret"
`), 0o644)
	require.NoError(t, err)

	cfg, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, Default().Server.Port, cfg.Server.Port)
	require.Equal(t, Default().Database.URL, cfg.Database.URL)
	require.Equal(t, "secret", cfg.GitHub.Token)
	require.Equal(t, Default().Mail.Port, cfg.Mail.Port)
	require.Equal(t, Default().Mail.ApiBaseUrl, cfg.Mail.ApiBaseUrl)
	require.Equal(t, Default().Logging.Level, cfg.Logging.Level)
	require.Equal(t, Default().Logging.Format, cfg.Logging.Format)
}

func TestLoad_ReturnsErrorForInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	err := os.WriteFile(path, []byte("server: ["), 0o644)
	require.NoError(t, err)

	_, err = Load(path)
	require.Error(t, err)
}

func TestLoad_AppliesMailDefaultsWhenOnlyMailSectionExists(t *testing.T) {
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
	require.Equal(t, Default().Mail.Port, cfg.Mail.Port)
	require.Equal(t, Default().Mail.ApiBaseUrl, cfg.Mail.ApiBaseUrl)
	require.Equal(t, Default().Logging.Level, cfg.Logging.Level)
	require.Equal(t, Default().Logging.Format, cfg.Logging.Format)
}

func TestLoad_UsesDefaultPathWhenEmptyPathProvided(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)

	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
	})

	err = os.WriteFile("config.yaml", []byte(`
server:
  port: "9191"
`), 0o644)
	require.NoError(t, err)

	cfg, err := Load("")
	require.NoError(t, err)
	require.Equal(t, "9191", cfg.Server.Port)
}
