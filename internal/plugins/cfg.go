package plugins

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

/*
Config accessors.

Plugin config blocks arrive as map[string]any straight from yaml.v3 (or from
the GUI's editors), so the same key may be an int, an int64, a float64, a bool
or a string depending on how the user typed it. The Python plugins read them
with config.get(key, default) followed by int()/.lower()/truthiness; these
helpers reproduce that tolerance:

  - a missing key yields the default;
  - a key present with a null value is treated as Python treated None — the
    default for strings and ints (where Python would have raised), false for
    booleans (Python truthiness);
  - numbers are accepted for booleans (non-zero is true) and strings are
    accepted for numbers (Python int("10")).
*/

// getString returns cfg[key] as a string, or def when missing or null.
// Non-string scalars are formatted with fmt.Sprint.
func getString(cfg map[string]any, key, def string) string {
	v, ok := cfg[key]
	if !ok || v == nil {
		return def
	}
	if s, ok := v.(string); ok {
		return s
	}
	switch t := v.(type) {
	case bool, int, int64, int32, uint, uint64, float64, float32:
		return fmt.Sprint(t)
	}
	return def
}

// getInt returns cfg[key] as an int the way Python's int(value) would,
// truncating floats and parsing decimal strings. Unparsable values and nulls
// yield def.
func getInt(cfg map[string]any, key string, def int) int {
	v, ok := cfg[key]
	if !ok || v == nil {
		return def
	}
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case int32:
		return int(t)
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return def
		}
		return int(t)
	case float32:
		return int(t)
	case bool:
		if t {
			return 1
		}
		return 0
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return def
		}
		return n
	}
	return def
}

// getBool returns cfg[key] as a bool. A missing key yields def; a null value
// is false (Python truthiness of None). Strings are parsed like
// strconv.ParseBool plus yes/no/on/off, falling back to Python truthiness
// (non-empty is true) when they do not parse. Numbers are true when non-zero.
func getBool(cfg map[string]any, key string, def bool) bool {
	v, ok := cfg[key]
	if !ok {
		return def
	}
	return truthy(v)
}

// truthy evaluates an arbitrary YAML scalar the way the Python plugins did
// when they wrote `if config.get(...)`.
func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case int:
		return t != 0
	case int64:
		return t != 0
	case int32:
		return t != 0
	case uint:
		return t != 0
	case uint64:
		return t != 0
	case float64:
		return t != 0
	case float32:
		return t != 0
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		switch s {
		case "1", "t", "true", "y", "yes", "on":
			return true
		case "0", "f", "false", "n", "no", "off", "":
			return false
		}
		return true
	case []any:
		return len(t) > 0
	case map[string]any:
		return len(t) > 0
	}
	return true
}

// getList returns cfg[key] as a []any, accepting []any and []string, or nil.
func getList(cfg map[string]any, key string) []any {
	v, ok := cfg[key]
	if !ok {
		return nil
	}
	return asList(v)
}

// asList coerces the list shapes yaml.v3 and the GUI produce to []any.
func asList(v any) []any {
	switch t := v.(type) {
	case []any:
		return t
	case []string:
		out := make([]any, len(t))
		for i, s := range t {
			out[i] = s
		}
		return out
	case []map[string]any:
		out := make([]any, len(t))
		for i, m := range t {
			out[i] = m
		}
		return out
	}
	return nil
}

// getStringList returns the string items of cfg[key] (used for `targets`).
// Non-string items are dropped.
func getStringList(cfg map[string]any, key string) []string {
	items := getList(cfg, key)
	out := make([]string, 0, len(items))
	for _, it := range items {
		if s, ok := it.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// asStringMap coerces the map shapes yaml.v3 can produce to map[string]any.
func asStringMap(v any) (map[string]any, bool) {
	switch t := v.(type) {
	case map[string]any:
		return t, true
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			ks, ok := k.(string)
			if !ok {
				continue
			}
			out[ks] = val
		}
		return out, true
	}
	return nil, false
}

// Term is one search term of a string_list field as the GUI edits it.
type Term struct {
	Term    string
	Enabled bool
}

/*
ParseTerms reads a string_list value in every shape the Python plugins
accepted (R5.3, R5.4):

  - a comma-separated string: split, trimmed, empties dropped, all enabled;
  - a list whose items are {"term": ..., "enabled": ...} maps (enabled
    defaults to true; items with an empty term are dropped) or bare strings
    (kept verbatim, as the Python code did — no trimming).

Anything else (null, a number) yields nil. Disabled terms are returned so the
GUI can show them; ParseQueries drops them.
*/
func ParseTerms(v any) []Term {
	var terms []Term
	switch t := v.(type) {
	case string:
		for _, part := range strings.Split(t, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				terms = append(terms, Term{Term: part, Enabled: true})
			}
		}
	default:
		for _, item := range asList(v) {
			if s, ok := item.(string); ok {
				terms = append(terms, Term{Term: s, Enabled: true})
				continue
			}
			m, ok := asStringMap(item)
			if !ok {
				continue
			}
			term := getString(m, "term", "")
			if term == "" {
				continue
			}
			terms = append(terms, Term{Term: term, Enabled: getBool(m, "enabled", true)})
		}
	}
	return terms
}

// ParseQueries is the Wallhaven _parse_queries semantics (R5.3): the enabled
// terms of v, or [fallback] when none remain.
func ParseQueries(v any, fallback string) []string {
	queries := enabledQueries(v)
	if len(queries) == 0 {
		return []string{fallback}
	}
	return queries
}

// enabledQueries is _parse_queries without the caller's fallback: it returns
// an empty (non-nil) slice when nothing is enabled, matching the golden
// table's `[]`.
func enabledQueries(v any) []string {
	queries := []string{}
	for _, t := range ParseTerms(v) {
		if t.Enabled {
			queries = append(queries, t.Term)
		}
	}
	return queries
}

// errMissingStore reports a plugin that was constructed without a store it
// needs. The Python plugins always opened their stores themselves; the Go
// port receives them through Deps and refuses to run without them rather
// than silently skipping the history and blacklist checks.
func errMissingStore(plugin, which string) error {
	return fmt.Errorf("%s: %s store not configured", plugin, which)
}
