//go:build unit

package logging

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew_CreatesJSONLoggerByDefault(t *testing.T) {
	logger, err := New("", "")
	require.NoError(t, err)
	require.NotNil(t, logger)
}

func TestNew_CreatesTextLogger(t *testing.T) {
	logger, err := New("debug", "text")
	require.NoError(t, err)
	require.NotNil(t, logger)
}

func TestNew_RejectsUnsupportedLevel(t *testing.T) {
	logger, err := New("trace", "json")
	require.Nil(t, logger)
	require.Error(t, err)
}

func TestNew_RejectsUnsupportedFormat(t *testing.T) {
	logger, err := New("info", "yaml")
	require.Nil(t, logger)
	require.Error(t, err)
}

func TestParseLevel_SupportsWarnAlias(t *testing.T) {
	level, err := parseLevel("warning")
	require.NoError(t, err)
	require.Equal(t, slog.LevelWarn, level)
}
