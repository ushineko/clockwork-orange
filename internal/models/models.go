/*
Package models is the Stable Diffusion model manager (spec 031 R6): a small
curated catalog, a download that resumes and verifies, and the on-disk layout.

It deliberately does not use malina's pkg/download. That package pulls in
go-getter and its cloud SDKs, which grew the CLI from 20 MB to 74 MB for a
catalog lookup and an HTTP GET (spec 031, "Findings"). The catalog below is
the single-file image models that malina's bundle list also carries, with the
SHA-256 that Hugging Face publishes for each file.
*/
package models

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/events"
)

// EnvDir overrides the models directory (tests, and users with a big disk).
const EnvDir = "CLOCKWORK_MODELS_DIR"

// File is one downloadable file.
type File struct {
	Name   string
	URL    string
	Size   int64
	SHA256 string
}

// Model is a catalog entry: one checkpoint the plugin can load.
type Model struct {
	Name        string
	Description string
	License     string
	// Gated models need a Hugging Face token in HF_TOKEN.
	Gated bool
	File  File
}

// Catalog lists the models the plugin supports, smallest first.
func Catalog() []Model {
	return []Model{
		{
			Name:        "sd-1.5",
			Description: "Stable Diffusion v1.5, the classic baseline (SD 1.x).",
			License:     "CreativeML Open RAIL-M",
			File: File{
				Name: "v1-5-pruned-emaonly.safetensors",
				URL: "https://huggingface.co/stable-diffusion-v1-5/stable-diffusion-v1-5/resolve/" +
					"451f4fe16113bff5a5d2269ed5ad43b0592e9a14/v1-5-pruned-emaonly.safetensors",
				Size:   4265146304,
				SHA256: "6ce0161689b3853acaa03779ec93eafe75a02f4ced659bee03f50797806fa2fa",
			},
		},
		{
			Name:        "sdxl-base-1.0",
			Description: "Stable Diffusion XL base 1.0, higher quality and slower.",
			License:     "CreativeML Open RAIL++-M",
			File: File{
				Name:   "sd_xl_base_1.0.safetensors",
				URL:    "https://huggingface.co/stabilityai/stable-diffusion-xl-base-1.0/resolve/main/sd_xl_base_1.0.safetensors",
				Size:   6938078334,
				SHA256: "31e35c80fc4829d14f90153f4c74cd59c90b779f6afe05a74cd6120b893f7e5b",
			},
		},
	}
}

// legacyIDs maps the Hugging Face ids the v2.9.5 plugin offered to catalog
// names. Only ids with a catalog equivalent are listed (spec 031 R3).
var legacyIDs = map[string]string{
	"runwayml/stable-diffusion-v1-5":                       "sd-1.5",
	"stable-diffusion-v1-5/stable-diffusion-v1-5":          "sd-1.5",
	"stabilityai/stable-diffusion-xl-base-1.0":             "sdxl-base-1.0",
	"stabilityai/stable-diffusion-xl-base-1.0-safetensors": "sdxl-base-1.0",
}

// Lookup finds a catalog model by name or by a v2.9.5 Hugging Face id.
func Lookup(name string) (Model, bool) {
	if mapped, ok := legacyIDs[name]; ok {
		name = mapped
	}
	for _, m := range Catalog() {
		if m.Name == name {
			return m, true
		}
	}
	return Model{}, false
}

// DefaultDir is where models live: the user's data directory, not the cache
// (a multi-gigabyte download should not vanish with a cache clean) and not
// ~/.config.
func DefaultDir() string {
	if v := os.Getenv(EnvDir); v != "" {
		return v
	}
	home := config.HomeDir()
	switch runtime.GOOS {
	case "windows":
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			return filepath.Join(v, "clockwork-orange", "models")
		}
		return filepath.Join(home, "AppData", "Local", "clockwork-orange", "models")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "clockwork-orange", "models")
	default:
		if v := os.Getenv("XDG_DATA_HOME"); v != "" {
			return filepath.Join(v, "clockwork-orange", "models")
		}
		return filepath.Join(home, ".local", "share", "clockwork-orange", "models")
	}
}

// Store is a models directory plus the HTTP client used to fill it.
type Store struct {
	Dir  string
	HTTP *http.Client
	// Token is sent as a bearer token; it is read from HF_TOKEN by NewStore.
	Token string
}

// NewStore returns a Store on DefaultDir with the default client.
func NewStore() Store {
	return Store{Dir: DefaultDir(), HTTP: http.DefaultClient, Token: os.Getenv("HF_TOKEN")}
}

// Path is where m's file lives (it may not exist).
func (s Store) Path(m Model) string { return filepath.Join(s.Dir, m.Name, m.File.Name) }

// Pulled reports whether m's file is present at its full size. It does not
// re-hash: Pull verifies before the file is moved into place.
func (s Store) Pulled(m Model) bool {
	st, err := os.Stat(s.Path(m))
	return err == nil && st.Mode().IsRegular() && st.Size() == m.File.Size
}

// ErrChecksum reports a download whose SHA-256 differs from the pinned one.
var ErrChecksum = errors.New("checksum mismatch")

// ErrGated reports a gated model pulled without a token.
var ErrGated = errors.New("this model is gated: accept its licence on Hugging Face and set HF_TOKEN")

// Pull downloads m into the store, resuming a partial file, and returns the
// final path. A pulled file is left alone. The file is verified against its
// pinned SHA-256 before it is renamed into place; a bad one is deleted.
func (s Store) Pull(ctx context.Context, m Model, ev events.Events) (string, error) {
	final := s.Path(m)
	if s.Pulled(m) {
		ev.Infof("[models] %s is already pulled", m.Name)
		return final, nil
	}
	if m.Gated && s.Token == "" {
		return "", ErrGated
	}
	if err := os.MkdirAll(filepath.Dir(final), 0o750); err != nil {
		return "", fmt.Errorf("create %s: %w", filepath.Dir(final), err)
	}
	part := final + ".part"
	if err := s.fetch(ctx, m, part, ev); err != nil {
		return "", err
	}
	ev.Progress(99, "Verifying "+m.File.Name+"...")
	sum, err := sha256File(ctx, part)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(sum, m.File.SHA256) {
		_ = os.Remove(part)
		return "", fmt.Errorf("%w for %s: got %s, want %s", ErrChecksum, m.File.Name, sum[:12], m.File.SHA256[:12])
	}
	if err := os.Rename(part, final); err != nil {
		return "", fmt.Errorf("install %s: %w", final, err)
	}
	ev.Progress(100, "Done!")
	return final, nil
}

// fetch appends to part until it holds the whole file.
func (s Store) fetch(ctx context.Context, m Model, part string, ev events.Events) error {
	var have int64
	if st, err := os.Stat(part); err == nil {
		have = st.Size()
	}
	if have > m.File.Size {
		_ = os.Remove(part)
		have = 0
	}
	if have < m.File.Size {
		if err := s.download(ctx, m, part, have, ev); err != nil {
			return err
		}
	}
	return nil
}

func (s Store) download(ctx context.Context, m Model, part string, have int64, ev events.Events) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.File.URL, nil)
	if err != nil {
		return fmt.Errorf("download %s: %w", m.File.Name, err)
	}
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
		ev.Infof("[models] resuming %s at %d MiB", m.File.Name, have>>20)
	}
	if s.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.Token)
	}
	client := s.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", m.File.Name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	flags := os.O_CREATE | os.O_WRONLY
	switch resp.StatusCode {
	case http.StatusPartialContent:
		flags |= os.O_APPEND
	case http.StatusOK:
		flags |= os.O_TRUNC // the server ignored Range: start over
		have = 0
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("download %s: %s: %w", m.File.Name, resp.Status, ErrGated)
	case http.StatusRequestedRangeNotSatisfiable:
		_ = os.Remove(part)
		return fmt.Errorf("download %s: stale partial file removed, retry", m.File.Name)
	default:
		return fmt.Errorf("download %s: %s", m.File.Name, resp.Status)
	}
	out, err := os.OpenFile(part, flags, 0o600) //nolint:gosec // path built from the catalog
	if err != nil {
		return fmt.Errorf("open %s: %w", part, err)
	}
	if err := copyWithProgress(ctx, out, resp.Body, have, m, ev); err != nil {
		_ = out.Close() // the partial file stays for the next resume
		return err
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %s: %w", part, err)
	}
	return nil
}

func copyWithProgress(ctx context.Context, dst io.Writer, src io.Reader, have int64, m Model, ev events.Events) error {
	buf := make([]byte, 256<<10)
	last := time.Time{}
	lastPct := -1
	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			if _, err := dst.Write(buf[:n]); err != nil {
				return fmt.Errorf("write %s: %w", m.File.Name, err)
			}
			have += int64(n)
			pct := int(have * 98 / m.File.Size) // 98: leave room for verification
			if pct != lastPct && time.Since(last) > 100*time.Millisecond {
				ev.Progress(pct, fmt.Sprintf("Downloading %s (%d of %d MiB)", m.File.Name, have>>20, m.File.Size>>20))
				lastPct, last = pct, time.Now()
			}
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return fmt.Errorf("download %s: %w", m.File.Name, rerr)
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("download %s: %w", m.File.Name, err)
		}
	}
}

func sha256File(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // path built from the catalog
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	buf := make([]byte, 1<<20)
	for {
		n, rerr := f.Read(buf)
		h.Write(buf[:n])
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", fmt.Errorf("read %s: %w", path, rerr)
		}
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("verify %s: %w", path, err)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
