package plugins

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/events"
)

// local is the port of plugins/local.py (R5.2): it validates a configured
// file or directory path and hands it back; the engine does the file-vs-dir
// work.
type local struct {
	deps Deps
}

func newLocal(d Deps) *local { return &local{deps: d} }

func (*local) Name() string { return "local" }

func (*local) Description() string {
	return "Returns a configured local file or directory path."
}

// Schema is plugins/local.py get_config_schema, in declaration order.
func (*local) Schema() []Field {
	return []Field{
		{
			Key:         "path",
			Type:        TypeString,
			Description: "Path to file or directory",
			Required:    true,
			Widget:      WidgetDirectoryPath,
		},
		{
			Key:         "recursive",
			Type:        TypeBoolean,
			Description: "Search recursively (if path is directory)",
			Default:     false,
		},
	}
}

// errMissingPath is the Python "Missing 'path' in configuration" result.
var errMissingPath = errors.New("Missing 'path' in configuration") //nolint:staticcheck // ST1005: the Python plugin's message text (R5.2)

// Run follows plugins/local.py run() step for step: a missing path is an
// error even for the process_blacklist action (the Python method checked
// path first), the action runs before the existence check, a nonexistent
// path is an error, otherwise the absolute, ~-expanded, symlink-resolved
// path is returned.
func (p *local) Run(_ context.Context, cfg map[string]any, ev events.Events) (Result, error) {
	pathStr := getString(cfg, "path", "")
	if pathStr == "" {
		return Result{}, errMissingPath
	}
	path := resolvePath(pathStr)

	if getString(cfg, "action", "") == "process_blacklist" {
		return processBlacklist(cfg, p.deps, "local", "[Local]", ev)
	}

	if _, err := os.Stat(path); err != nil {
		return Result{}, fmt.Errorf("Path not found: %s", path) //nolint:staticcheck // ST1005: the Python plugin's message text (R5.2)
	}
	return Result{Path: path}, nil
}

// resolvePath is Path(p).expanduser().resolve() with strict=False: the
// leading ~ is expanded, the path made absolute, and symlinks resolved when
// the path exists; a path that does not exist is returned absolute as-is.
func resolvePath(p string) string {
	p = config.ExpandPath(p)
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = p
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}
