package core

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/imaging"
	"github.com/ushineko/clockwork-orange/internal/store"
)

// Check is one self-test probe (R6.6).
type Check struct {
	Name   string
	OK     bool
	Detail string
}

// SelfTestResult is the whole run; OK is the AND of every check.
type SelfTestResult struct {
	Checks []Check
	OK     bool
}

// selfTestURL is the connectivity probe target, as in the Python.
const selfTestURL = "https://www.google.com"

// SelfTestRequest allows tests to skip the network probe.
type SelfTestRequest struct {
	Request
	SkipNetwork bool
}

// SelfTest verifies the environment (R6.6): runtime facts, SQLite, YAML,
// image codecs, platform tools, network, and the plugin registry. It never
// opens the user's real databases.
func SelfTest(ctx context.Context, req SelfTestRequest) SelfTestResult {
	var res SelfTestResult
	add := func(name string, err error, detail string) {
		c := Check{Name: name, OK: err == nil, Detail: detail}
		if err != nil {
			c.Detail = err.Error()
		}
		res.Checks = append(res.Checks, c)
	}
	exe, _ := os.Executable()
	add("runtime", nil, fmt.Sprintf("%s %s/%s %s", runtime.Version(), runtime.GOOS, runtime.GOARCH, exe))

	tmp, err := os.MkdirTemp("", "clockwork-selftest-*")
	if err != nil {
		add("tempdir", err, "")
	} else {
		defer func() { _ = os.RemoveAll(tmp) }()
		h, err := store.OpenHistory(filepath.Join(tmp, "history.db"))
		if err == nil {
			_, err = h.Stats()
			_ = h.Close()
		}
		add("sqlite", err, "open + query in a temp dir")
	}

	doc := config.Defaults()
	doc.Plugins["local"] = map[string]any{"enabled": true, "path": "~/Pictures"}
	b, err := config.Marshal(doc)
	if err == nil {
		_, err = config.Parse(b)
	}
	add("yaml", err, "marshal + parse round-trip")

	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	err = png.Encode(&buf, img)
	if err == nil {
		_, _, err = imaging.Decode(&buf)
	}
	add("image codecs", err, "png encode/decode; jpeg, gif, bmp, tiff, webp registered")

	if runtime.GOOS == "linux" {
		for _, tool := range []string{"qdbus6", "kwriteconfig6", "systemctl"} {
			p, err := exec.LookPath(tool)
			add(tool, err, p)
		}
	}

	if !req.SkipNetwork {
		nctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		r, err := http.NewRequestWithContext(nctx, http.MethodHead, selfTestURL, nil)
		var resp *http.Response
		if err == nil {
			resp, err = http.DefaultClient.Do(r)
			if err == nil {
				_ = resp.Body.Close()
			}
		}
		add("network", err, "HEAD "+selfTestURL)
	}

	names := AvailablePluginNames()
	var regErr error
	if len(names) != 3 {
		regErr = fmt.Errorf("expected 3 plugins, found %d", len(names))
	}
	add("plugins", regErr, fmt.Sprintf("%v", names))

	res.OK = true
	for _, c := range res.Checks {
		res.OK = res.OK && c.OK
	}
	return res
}
