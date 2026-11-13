package models

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/events"
)

// server serves body with Range support and counts requests.
func server(t *testing.T, body []byte) (*httptest.Server, *atomic.Int32, *atomic.Value) {
	t.Helper()
	var hits atomic.Int32
	var lastRange atomic.Value
	lastRange.Store("")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		rng := r.Header.Get("Range")
		lastRange.Store(rng)
		if rng == "" {
			_, _ = w.Write(body)
			return
		}
		from, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rng, "bytes="), "-"))
		if err != nil || from > len(body) {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body[from:])
	}))
	t.Cleanup(ts.Close)
	return ts, &hits, &lastRange
}

func modelFor(url string, body []byte) Model {
	sum := sha256.Sum256(body)
	return Model{Name: "tiny", License: "MIT", File: File{
		Name: "tiny.safetensors", URL: url, Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:]),
	}}
}

func body() []byte { return []byte(strings.Repeat("0123456789abcdef", 4096)) }

func TestPullDownloadsAndVerifies(t *testing.T) {
	b := body()
	ts, _, _ := server(t, b)
	s := Store{Dir: t.TempDir(), HTTP: ts.Client()}
	m := modelFor(ts.URL, b)

	var pcts []int
	path, err := s.Pull(context.Background(), m, events.Events{OnProgress: func(p int, _ string) { pcts = append(pcts, p) }})
	require.NoError(t, err)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, b, got)
	require.True(t, s.Pulled(m))
	require.NoFileExists(t, path+".part")
	require.Equal(t, 100, pcts[len(pcts)-1])
}

func TestPullSkipsAPulledModel(t *testing.T) {
	b := body()
	ts, hits, _ := server(t, b)
	s := Store{Dir: t.TempDir(), HTTP: ts.Client()}
	m := modelFor(ts.URL, b)
	_, err := s.Pull(context.Background(), m, events.Events{})
	require.NoError(t, err)
	before := hits.Load()
	_, err = s.Pull(context.Background(), m, events.Events{})
	require.NoError(t, err)
	require.Equal(t, before, hits.Load(), "a pulled model is not fetched again")
}

func TestPullResumesAPartialFile(t *testing.T) {
	b := body()
	ts, _, lastRange := server(t, b)
	s := Store{Dir: t.TempDir(), HTTP: ts.Client()}
	m := modelFor(ts.URL, b)
	require.NoError(t, os.MkdirAll(filepath.Dir(s.Path(m)), 0o750))
	require.NoError(t, os.WriteFile(s.Path(m)+".part", b[:1000], 0o600))

	path, err := s.Pull(context.Background(), m, events.Events{})
	require.NoError(t, err)
	require.Equal(t, "bytes=1000-", lastRange.Load())
	got, _ := os.ReadFile(path)
	require.Equal(t, b, got)
}

func TestPullRejectsAndRemovesABadChecksum(t *testing.T) {
	b := body()
	ts, _, _ := server(t, b)
	s := Store{Dir: t.TempDir(), HTTP: ts.Client()}
	m := modelFor(ts.URL, b)
	m.File.SHA256 = strings.Repeat("0", 64)

	_, err := s.Pull(context.Background(), m, events.Events{})
	require.ErrorIs(t, err, ErrChecksum)
	require.NoFileExists(t, s.Path(m))
	require.NoFileExists(t, s.Path(m)+".part")
	require.False(t, s.Pulled(m))
}

func TestPullKeepsThePartialFileWhenCancelled(t *testing.T) {
	b := body()
	ts, _, _ := server(t, b)
	s := Store{Dir: t.TempDir(), HTTP: ts.Client()}
	m := modelFor(ts.URL, b)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.Pull(ctx, m, events.Events{})
	require.Error(t, err)
	require.False(t, s.Pulled(m))
}

func TestGatedModelNeedsAToken(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	m := Model{Name: "g", Gated: true, File: File{Name: "g.bin", Size: 1}}
	_, err := s.Pull(context.Background(), m, events.Events{})
	require.ErrorIs(t, err, ErrGated)
}

func TestLookupAcceptsLegacyHuggingFaceIDs(t *testing.T) {
	for _, id := range []string{"sd-1.5", "runwayml/stable-diffusion-v1-5", "stable-diffusion-v1-5/stable-diffusion-v1-5"} {
		m, ok := Lookup(id)
		require.True(t, ok, id)
		require.Equal(t, "sd-1.5", m.Name)
	}
	_, ok := Lookup("prompthero/openjourney")
	require.False(t, ok)
}

func TestCatalogEntriesArePinned(t *testing.T) {
	for _, m := range Catalog() {
		require.Len(t, m.File.SHA256, 64, m.Name)
		require.Positive(t, m.File.Size, m.Name)
		require.NotEmpty(t, m.License, m.Name)
		require.True(t, strings.HasPrefix(m.File.URL, "https://"), m.Name)
	}
}

func TestDefaultDirHonoursTheEnvOverride(t *testing.T) {
	t.Setenv(EnvDir, "/somewhere")
	require.Equal(t, "/somewhere", DefaultDir())
}
