package models

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/events"
)

// tinyModel is a real 4.9 MB file on Hugging Face (MIT licence, the Tiny
// AutoEncoder), small enough to download in a test. The catalog's own files are
// 4 GB and up. The hash is the one Hugging Face publishes for it.
func tinyModel() Model {
	return Model{Name: "taesd", License: "MIT", File: File{
		Name:   "taesd_decoder.safetensors",
		URL:    "https://huggingface.co/madebyollin/taesd/resolve/main/taesd_decoder.safetensors",
		Size:   4895612,
		SHA256: "f0fb51dd10d41c26612c070fa0b52ea0215a5ff90792134b4971109dd713c019",
	}}
}

// TestLivePull drives the real download path against Hugging Face (spec 031 R6,
// the integration-boundary criterion): a fresh pull, a pull resumed from a
// truncated partial file, and a pull whose hash is wrong. Gated by
// CLOCKWORK_LIVE_NET=1 like the other live tests.
func TestLivePull(t *testing.T) {
	if os.Getenv("CLOCKWORK_LIVE_NET") != "1" {
		t.Skip("set CLOCKWORK_LIVE_NET=1 to run against Hugging Face")
	}
	ctx := context.Background()
	m := tinyModel()

	t.Run("fresh pull verifies and installs", func(t *testing.T) {
		s := Store{Dir: t.TempDir(), HTTP: nil}
		path, err := s.Pull(ctx, m, events.Events{})
		require.NoError(t, err)
		require.True(t, s.Pulled(m))
		require.Equal(t, s.Path(m), path)
		require.NoFileExists(t, path+".part")
	})

	t.Run("a truncated partial file is resumed", func(t *testing.T) {
		s := Store{Dir: t.TempDir()}
		path, err := s.Pull(ctx, m, events.Events{})
		require.NoError(t, err)

		// Turn the finished file back into a 2 MiB partial one.
		require.NoError(t, os.Rename(path, path+".part"))
		require.NoError(t, os.Truncate(path+".part", 2<<20))
		require.False(t, s.Pulled(m))

		var logs []string
		ev := events.Events{OnLog: func(_ events.Level, msg string) { logs = append(logs, msg) }}
		_, err = s.Pull(ctx, m, ev)
		require.NoError(t, err)
		require.True(t, s.Pulled(m), "the resumed file passes its checksum")
		require.True(t, anyContains(logs, "resuming"), "the pull resumed instead of starting over: %v", logs)
	})

	t.Run("a wrong hash is rejected and removed", func(t *testing.T) {
		s := Store{Dir: t.TempDir()}
		bad := m
		bad.File.SHA256 = strings.Repeat("0", 64)
		_, err := s.Pull(ctx, bad, events.Events{})
		require.ErrorIs(t, err, ErrChecksum)
		require.NoFileExists(t, s.Path(bad))
		require.NoFileExists(t, s.Path(bad)+".part")
		entries, _ := os.ReadDir(filepath.Dir(s.Path(bad)))
		require.Empty(t, entries, "nothing is left behind")
	})
}

func anyContains(lines []string, sub string) bool {
	for _, l := range lines {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}
