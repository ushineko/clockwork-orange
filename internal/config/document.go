package config

import (
	"bytes"
	"fmt"
	"maps"
	"math"
	"sort"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Top-level YAML keys of the v2.9.5 schema (R2.2). Anything not listed here
// lands in Document.Extra and is written back untouched.
const (
	keyDesktop             = "desktop"
	keyLockscreen          = "lockscreen"
	keyDualWallpapers      = "dual_wallpapers"
	keyDefaultWait         = "default_wait"
	keyDefaultURL          = "default_url"
	keyDefaultFile         = "default_file"
	keyDefaultDirectory    = "default_directory"
	keyConsoleFontFamily   = "console_font_family"
	keyConsoleFontSize     = "console_font_size"
	keyWindowWidth         = "window_width"
	keyWindowHeight        = "window_height"
	keyImageExtensions     = "image_extensions"
	keyDebug               = "debug"
	keyAutostart           = "autostart"
	keyRestartDelay        = "restart_delay"
	keyLogsRefreshInterval = "logs_refresh_interval"
	keyAutoUpdateLogs      = "auto_update_logs"
	keyPlugins             = "plugins"
)

// Wait intervals, in seconds, applied when the file sets none (R2.2, R2.5).
const (
	// DefaultWaitGUI is what the settings widget shows for an unset default_wait.
	DefaultWaitGUI = 300
	// DefaultWaitService is the daemon's fallback when neither --wait nor
	// default_wait is given.
	DefaultWaitService = 900
)

// DefaultImageExtensions is the GUI default for image_extensions (R2.2).
const DefaultImageExtensions = ".jpg,.jpeg,.png,.bmp,.gif,.tiff,.webp,.svg"

/*
Document is the typed view of ~/.config/clockwork-orange.yml (R2.2).

Zero values mean "unset". Scalar keys are written back when they are non-zero
or when the loaded file spelled them out explicitly (a `desktop: false` written
by the 2.9.x GUI survives a round trip), which is how the Python writer behaved:
it only ever emitted keys something had set. auto_update_logs is the exception
and is omitted whenever false.

Plugins holds every `plugins.<name>` block, known to this build or not, and
Extra every unknown top-level key; both go back to disk verbatim (D6, DV7).
*/
type Document struct {
	Desktop, Lockscreen, DualWallpapers bool
	DefaultWait                         int // 0 = unset
	DefaultURL, DefaultFile             string
	DefaultDirectory                    string
	ConsoleFontFamily                   string
	ConsoleFontSize                     int
	WindowWidth, WindowHeight           int
	ImageExtensions                     string
	Debug, Autostart                    bool
	RestartDelay, LogsRefreshInterval   int
	AutoUpdateLogs                      bool
	Plugins                             map[string]map[string]any // every plugin block, known or not; preserved verbatim
	Extra                               map[string]any            // unknown top-level keys, preserved verbatim

	// explicitZero records scalar keys the source document spelled out with a
	// zero value, so Marshal writes them back instead of dropping them. It is
	// allocated lazily and stays nil for documents built in Go.
	explicitZero map[string]bool
}

// Defaults is the document the GUI shows for a missing file (R2.2).
func Defaults() Document {
	return Document{
		DefaultWait:         DefaultWaitGUI,
		ConsoleFontFamily:   "Monospace",
		ConsoleFontSize:     10,
		WindowWidth:         800,
		WindowHeight:        600,
		ImageExtensions:     DefaultImageExtensions,
		RestartDelay:        10,
		LogsRefreshInterval: 5,
		Plugins:             map[string]map[string]any{},
		Extra:               map[string]any{},
	}
}

// Parse decodes YAML into a Document. No migration, no I/O (R2.2).
func Parse(b []byte) (Document, error) {
	raw, err := parseRaw(b)
	if err != nil {
		return Document{}, err
	}
	return fromRaw(raw)
}

// parseRaw decodes the top-level YAML mapping. An empty file is an empty map,
// matching Python's `yaml.safe_load(f) or {}`.
func parseRaw(b []byte) (map[string]any, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if raw == nil {
		raw = map[string]any{}
	}
	return raw, nil
}

// fromRaw builds the typed document from the decoded mapping.
func fromRaw(raw map[string]any) (Document, error) {
	d := Document{
		Plugins: map[string]map[string]any{},
		Extra:   map[string]any{},
	}
	var err error
	for key, v := range raw {
		switch key {
		case keyDesktop:
			d.Desktop, err = asBool(key, v)
		case keyLockscreen:
			d.Lockscreen, err = asBool(key, v)
		case keyDualWallpapers:
			d.DualWallpapers, err = asBool(key, v)
		case keyDefaultWait:
			d.DefaultWait, err = asInt(key, v)
		case keyDefaultURL:
			d.DefaultURL = asString(v)
		case keyDefaultFile:
			d.DefaultFile = asString(v)
		case keyDefaultDirectory:
			d.DefaultDirectory = asString(v)
		case keyConsoleFontFamily:
			d.ConsoleFontFamily = asString(v)
		case keyConsoleFontSize:
			d.ConsoleFontSize, err = asInt(key, v)
		case keyWindowWidth:
			d.WindowWidth, err = asInt(key, v)
		case keyWindowHeight:
			d.WindowHeight, err = asInt(key, v)
		case keyImageExtensions:
			d.ImageExtensions = asString(v)
		case keyDebug:
			d.Debug, err = asBool(key, v)
		case keyAutostart:
			d.Autostart, err = asBool(key, v)
		case keyRestartDelay:
			d.RestartDelay, err = asInt(key, v)
		case keyLogsRefreshInterval:
			d.LogsRefreshInterval, err = asInt(key, v)
		case keyAutoUpdateLogs:
			d.AutoUpdateLogs, err = asBool(key, v)
		case keyPlugins:
			d.Plugins, err = asPlugins(v)
		default:
			d.Extra[key] = v
			continue
		}
		if err != nil {
			return Document{}, err
		}
		zero := isZeroScalar(v)
		if key == keyPlugins {
			zero = len(d.Plugins) == 0
		}
		if key != keyAutoUpdateLogs && zero {
			if d.explicitZero == nil {
				d.explicitZero = map[string]bool{}
			}
			d.explicitZero[key] = true
		}
	}
	return d, nil
}

// asPlugins types the `plugins` mapping. A block with no body (`local:`) is an
// empty block; anything else that is not a mapping is a malformed file.
func asPlugins(v any) (map[string]map[string]any, error) {
	out := map[string]map[string]any{}
	if v == nil {
		return out, nil
	}
	blocks, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("config: %s: expected a mapping, got %T", keyPlugins, v)
	}
	for name, block := range blocks {
		switch b := block.(type) {
		case nil:
			out[name] = map[string]any{}
		case map[string]any:
			out[name] = b
		default:
			return nil, fmt.Errorf("config: %s.%s: expected a mapping, got %T", keyPlugins, name, block)
		}
	}
	return out, nil
}

func asBool(key string, v any) (bool, error) {
	switch b := v.(type) {
	case nil:
		return false, nil
	case bool:
		return b, nil
	default:
		return false, fmt.Errorf("config: %s: expected a boolean, got %T", key, v)
	}
}

// asInt accepts the integer spellings yaml.v3 produces plus a numeric string,
// which is what Python's `int(config["default_wait"])` tolerated.
func asInt(key string, v any) (int, error) {
	switch n := v.(type) {
	case nil:
		return 0, nil
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case uint64:
		if n > math.MaxInt {
			return 0, fmt.Errorf("config: %s: %d overflows int", key, n)
		}
		return int(n), nil
	case float64:
		if n != float64(int(n)) {
			return 0, fmt.Errorf("config: %s: expected an integer, got %v", key, n)
		}
		return int(n), nil
	case string:
		i, err := strconv.Atoi(n)
		if err != nil {
			return 0, fmt.Errorf("config: %s: expected an integer, got %q", key, n)
		}
		return i, nil
	default:
		return 0, fmt.Errorf("config: %s: expected an integer, got %T", key, v)
	}
}

// asString renders any scalar as text; Python code paths did str() on these.
func asString(v any) string {
	switch s := v.(type) {
	case nil:
		return ""
	case string:
		return s
	default:
		return fmt.Sprint(v)
	}
}

// isZeroScalar reports whether a decoded scalar is the zero value of its type.
func isZeroScalar(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case bool:
		return !x
	case int:
		return x == 0
	case int64:
		return x == 0
	case uint64:
		return x == 0
	case float64:
		return x == 0
	case string:
		return x == ""
	default:
		return false
	}
}

// Marshal renders the bytes Save writes: sorted keys, 2-space indent (R2.3).
func Marshal(d Document) ([]byte, error) {
	node, err := toNode(d.toRaw())
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(node); err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}
	return buf.Bytes(), nil
}

// toRaw flattens the document to the mapping that is written to disk,
// applying the omission rules described on Document.
func (d Document) toRaw() map[string]any {
	out := make(map[string]any, len(d.Extra)+18)
	maps.Copy(out, d.Extra)
	d.putBool(out, keyDesktop, d.Desktop)
	d.putBool(out, keyLockscreen, d.Lockscreen)
	d.putBool(out, keyDualWallpapers, d.DualWallpapers)
	d.putInt(out, keyDefaultWait, d.DefaultWait)
	d.putString(out, keyDefaultURL, d.DefaultURL)
	d.putString(out, keyDefaultFile, d.DefaultFile)
	d.putString(out, keyDefaultDirectory, d.DefaultDirectory)
	d.putString(out, keyConsoleFontFamily, d.ConsoleFontFamily)
	d.putInt(out, keyConsoleFontSize, d.ConsoleFontSize)
	d.putInt(out, keyWindowWidth, d.WindowWidth)
	d.putInt(out, keyWindowHeight, d.WindowHeight)
	d.putString(out, keyImageExtensions, d.ImageExtensions)
	d.putBool(out, keyDebug, d.Debug)
	d.putBool(out, keyAutostart, d.Autostart)
	d.putInt(out, keyRestartDelay, d.RestartDelay)
	d.putInt(out, keyLogsRefreshInterval, d.LogsRefreshInterval)
	if d.AutoUpdateLogs { // omitted when false (R2.2)
		out[keyAutoUpdateLogs] = true
	}
	if len(d.Plugins) > 0 || d.explicitZero[keyPlugins] {
		plugins := make(map[string]any, len(d.Plugins))
		for name, block := range d.Plugins {
			plugins[name] = block
		}
		out[keyPlugins] = plugins
	}
	return out
}

func (d Document) putBool(out map[string]any, key string, v bool) {
	if v || d.explicitZero[key] {
		out[key] = v
	}
}

func (d Document) putInt(out map[string]any, key string, v int) {
	if v != 0 || d.explicitZero[key] {
		out[key] = v
	}
}

func (d Document) putString(out map[string]any, key string, v string) {
	if v != "" || d.explicitZero[key] {
		out[key] = v
	}
}

// toNode converts decoded YAML data to a node tree whose mappings are sorted
// by key, byte-wise, like Python's `sort_keys=True`. yaml.v3 would sort map
// keys on its own, but with a natural ("item9" < "item10") order that the
// Python writer never produced.
func toNode(v any) (*yaml.Node, error) {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, k := range keys {
			val, err := toNode(x[k])
			if err != nil {
				return nil, err
			}
			n.Content = append(n.Content, scalarNode(k), val)
		}
		return n, nil
	case map[any]any:
		// Non-string keys only appear in hand-edited files; order them by
		// their text so the output is at least deterministic.
		keys := make([]any, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j]) })
		n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, k := range keys {
			key, err := toNode(k)
			if err != nil {
				return nil, err
			}
			val, err := toNode(x[k])
			if err != nil {
				return nil, err
			}
			n.Content = append(n.Content, key, val)
		}
		return n, nil
	case []any:
		n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, item := range x {
			val, err := toNode(item)
			if err != nil {
				return nil, err
			}
			n.Content = append(n.Content, val)
		}
		return n, nil
	default:
		var n yaml.Node
		if err := n.Encode(v); err != nil {
			return nil, fmt.Errorf("encode config value %v: %w", v, err)
		}
		return &n, nil
	}
}

func scalarNode(s string) *yaml.Node {
	var n yaml.Node
	_ = n.Encode(s) // encoding a string cannot fail
	return &n
}

// PluginEnabled reports whether plugins.<name>.enabled is truthy, with
// Python's notion of truth: a non-empty string or non-zero number counts.
func (d Document) PluginEnabled(name string) bool {
	block, ok := d.Plugins[name]
	if !ok {
		return false
	}
	return truthy(block["enabled"])
}

// EnabledPlugins lists the enabled plugin names, sorted.
func (d Document) EnabledPlugins() []string {
	var names []string
	for name := range d.Plugins {
		if d.PluginEnabled(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// SetPlugin replaces plugins.<name> wholesale, the way the GUI auto-save
// wrote each plugin widget's config to its own key.
func (d *Document) SetPlugin(name string, block map[string]any) {
	if d.Plugins == nil {
		d.Plugins = map[string]map[string]any{}
	}
	d.Plugins[name] = block
}

// truthy mirrors Python's bool() for the values yaml.v3 decodes.
func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case int:
		return x != 0
	case int64:
		return x != 0
	case uint64:
		return x != 0
	case float64:
		return x != 0
	case string:
		return x != ""
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	default:
		return true
	}
}
