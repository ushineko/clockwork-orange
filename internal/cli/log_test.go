package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/events"
)

// Log lines carry the Python `[LEVEL]` prefixes, --log-level hides the
// quieter ones, and the two plugin protocol markers print regardless (R6.7).
func TestEventsRenderPythonPrefixesAndProtocolMarkers(t *testing.T) {
	var buf bytes.Buffer
	ev := newEvents(&buf, events.LevelInfo)
	ev.Debugf("hidden %d", 1)
	ev.Infof("shown")
	ev.Warnf("careful")
	ev.Errorf("broken")
	ev.Progress(95, "Cleaning up old files...")
	ev.ImageSaved("/tmp/x.jpg")
	require.Equal(t,
		"[INFO] shown\n[WARN] careful\n[ERROR] broken\n::PROGRESS:: 95 :: Cleaning up old files...\n::IMAGE_SAVED:: /tmp/x.jpg\n",
		buf.String())

	buf.Reset()
	newEvents(&buf, events.LevelDebug).Debugf("now visible")
	require.Equal(t, "[DEBUG] now visible\n", buf.String())
}

func TestLogLevelWordsIncludeTheWarningAlias(t *testing.T) {
	for word, want := range map[string]events.Level{
		"debug": events.LevelDebug, "info": events.LevelInfo, "warn": events.LevelWarn,
		"warning": events.LevelWarn, "error": events.LevelError,
	} {
		got, err := parseLevel(word)
		require.NoError(t, err)
		require.Equal(t, want, got, word)
	}
	_, err := parseLevel("loud")
	var usage *UsageError
	require.ErrorAs(t, err, &usage)
}
