/*
Package plugins holds the in-process wallpaper sources (spec 010 R5.1–R5.4,
decision D3): local, wallhaven and duckduckgo_images.

Each plugin is a faithful port of the corresponding plugins/*.py from v2.9.5.
The Python subprocess protocol (JSON on stdout, "::PROGRESS::" and
"::IMAGE_SAVED::" markers on stderr) is replaced by direct calls through
events.Events; everything else — schema keys and defaults, runtime keys,
per-item download flow, interval scheduling, retention, and the known quirks
listed in the spec's "Deliberate Deviations" preamble — is kept as-is.
*/
package plugins

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"time"

	"github.com/ushineko/clockwork-orange/internal/events"
	"github.com/ushineko/clockwork-orange/internal/store"
)

// FieldType is the type tag of a schema field (R5.1).
type FieldType string

// Schema field types, spelled exactly as the Python schemas did.
const (
	TypeString     FieldType = "string"
	TypeBoolean    FieldType = "boolean"
	TypeInteger    FieldType = "integer"
	TypeStringList FieldType = "string_list"
)

// Widget hints the GUI at a specialised editor for a string field (R5.1).
type Widget string

// Widget hints; the empty string means a plain text field.
const (
	WidgetNone          Widget = ""
	WidgetFilePath      Widget = "file_path"
	WidgetDirectoryPath Widget = "directory_path"
)

// Field is one entry of a plugin's configuration schema (R5.1). The order of
// a Schema() slice is the order the Python dict literal declared its keys.
type Field struct {
	Key         string
	Type        FieldType
	Description string
	Default     any
	Required    bool
	Enum        []string
	Suggestions []string
	Group       string
	Widget      Widget
}

// Result is what a successful Run returns (R5.1): Path is a file or
// directory the engine may draw wallpapers from; Message carries the
// human-readable outcome of an action such as process_blacklist.
type Result struct {
	Path    string
	Message string
}

// Plugin is a wallpaper source (R5.1).
type Plugin interface {
	Name() string
	Description() string
	Schema() []Field
	Run(ctx context.Context, cfg map[string]any, ev events.Events) (Result, error)
}

// Deps is everything a plugin needs from the outside world. Zero fields are
// filled with defaults by Registry: HTTP becomes an http.Client with no
// client-level timeout (per-request timeouts are applied via context), Now
// becomes time.Now. History and Blacklist have no default; a plugin that
// needs one and finds it nil returns an error from Run.
type Deps struct {
	History   *store.History
	Blacklist *store.Blacklist
	HTTP      *http.Client
	Now       func() time.Time
}

// withDefaults returns d with nil HTTP and Now replaced.
func (d Deps) withDefaults() Deps {
	if d.HTTP == nil {
		// A cookie jar, because the Python used one requests.Session for the
		// DuckDuckGo landing page and i.js and i.js wants the landing page's
		// cookie. cookiejar.New only errors on a bad options pointer.
		jar, _ := cookiejar.New(nil)
		d.HTTP = &http.Client{Jar: jar}
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return d
}

// Registry returns every built-in plugin in the order the Python plugin
// manager listed them (R5.1): local, wallhaven, duckduckgo_images.
func Registry(d Deps) []Plugin {
	d = d.withDefaults()
	return []Plugin{
		newLocal(d),
		newWallhaven(d),
		newDuckDuckGo(d),
	}
}

// Lookup finds a plugin by name.
func Lookup(reg []Plugin, name string) (Plugin, bool) {
	for _, p := range reg {
		if p.Name() == name {
			return p, true
		}
	}
	return nil, false
}

// Names lists plugin names in registry order.
func Names(reg []Plugin) []string {
	names := make([]string, 0, len(reg))
	for _, p := range reg {
		names = append(names, p.Name())
	}
	return names
}

// blacklistProcessed is the Message returned after a process_blacklist
// action, matching the Python plugins' result string.
const blacklistProcessed = "Blacklist processed"

// processBlacklist handles the shared `action: process_blacklist` runtime key
// (R5.1): every path in `targets` is added to the blacklist under the plugin's
// name and then deleted.
func processBlacklist(cfg map[string]any, d Deps, pluginName, logPrefix string, ev events.Events) (Result, error) {
	targets := getStringList(cfg, "targets")
	ev.Infof("%s Processing blacklist for %d files...", logPrefix, len(targets))
	if d.Blacklist == nil {
		return Result{}, errMissingStore(pluginName, "blacklist")
	}
	if _, err := d.Blacklist.ProcessFiles(targets, pluginName, ev); err != nil {
		return Result{}, err
	}
	return Result{Message: blacklistProcessed}, nil
}
