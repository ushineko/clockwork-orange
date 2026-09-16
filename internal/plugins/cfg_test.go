package plugins

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetStringTreatsMissingAndNullAsDefaultAndFormatsScalars(t *testing.T) {
	cfg := map[string]any{"s": "x", "n": nil, "i": 42, "b": true}
	require.Equal(t, "x", getString(cfg, "s", "d"))
	require.Equal(t, "d", getString(cfg, "missing", "d"))
	require.Equal(t, "d", getString(cfg, "n", "d"))
	require.Equal(t, "42", getString(cfg, "i", "d"))
	require.Equal(t, "true", getString(cfg, "b", "d"))
}

func TestGetIntAcceptsEveryNumericShapeYAMLProduces(t *testing.T) {
	cfg := map[string]any{
		"int": 7, "int64": int64(8), "float": 9.9, "str": " 10 ", "bad": "ten", "null": nil, "bool": true,
	}
	require.Equal(t, 7, getInt(cfg, "int", 1))
	require.Equal(t, 8, getInt(cfg, "int64", 1))
	require.Equal(t, 9, getInt(cfg, "float", 1), "int() truncates")
	require.Equal(t, 10, getInt(cfg, "str", 1))
	require.Equal(t, 1, getInt(cfg, "bad", 1))
	require.Equal(t, 1, getInt(cfg, "null", 1))
	require.Equal(t, 1, getInt(cfg, "bool", 5), "int(True) is 1")
	require.Equal(t, 1, getInt(cfg, "missing", 1))
}

func TestGetBoolFollowsPythonTruthinessWithStringParsing(t *testing.T) {
	cfg := map[string]any{
		"t": true, "f": false, "null": nil, "one": 1, "zero": 0, "float": 0.0,
		"sTrue": "True", "sfalse": "false", "sNo": "no", "sOn": "on", "sEmpty": "", "sWord": "maybe",
	}
	require.True(t, getBool(cfg, "t", false))
	require.False(t, getBool(cfg, "f", true))
	require.False(t, getBool(cfg, "null", true), "a present null is falsy, as None was")
	require.True(t, getBool(cfg, "missing", true))
	require.False(t, getBool(cfg, "missing", false))
	require.True(t, getBool(cfg, "one", false))
	require.False(t, getBool(cfg, "zero", true))
	require.False(t, getBool(cfg, "float", true))
	require.True(t, getBool(cfg, "sTrue", false))
	require.False(t, getBool(cfg, "sfalse", true))
	require.False(t, getBool(cfg, "sNo", true))
	require.True(t, getBool(cfg, "sOn", false))
	require.False(t, getBool(cfg, "sEmpty", true))
	require.True(t, getBool(cfg, "sWord", false), "non-empty unparsable string is truthy")
}

func TestGetStringListKeepsOnlyStrings(t *testing.T) {
	cfg := map[string]any{
		"targets": []any{"/a", 3, "/b", nil},
		"typed":   []string{"/c"},
		"scalar":  "/d",
	}
	require.Equal(t, []string{"/a", "/b"}, getStringList(cfg, "targets"))
	require.Equal(t, []string{"/c"}, getStringList(cfg, "typed"))
	require.Empty(t, getStringList(cfg, "scalar"))
	require.Empty(t, getStringList(cfg, "missing"))
}

func TestParseTermsReadsCommaStringsAndTermMapsAndBareStrings(t *testing.T) {
	require.Equal(t, []Term{{"a", true}, {"b", true}, {"c", true}}, ParseTerms("a, b ,c"))
	require.Nil(t, ParseTerms(""))
	require.Nil(t, ParseTerms(" , "))
	require.Nil(t, ParseTerms(nil))
	require.Nil(t, ParseTerms(42))

	list := []any{
		map[string]any{"term": "x", "enabled": true},
		map[string]any{"term": "y", "enabled": false},
		map[string]any{"term": "implicit"},
		map[string]any{"term": "", "enabled": true},
		map[string]any{"enabled": true},
		" z ",
		7,
	}
	require.Equal(t, []Term{
		{"x", true}, {"y", false}, {"implicit", true}, {" z ", true},
	}, ParseTerms(list), "disabled terms are kept for the GUI; bare strings are untrimmed as in Python")

	// yaml.v3 can hand back map[any]any for non-string-keyed maps.
	require.Equal(t, []Term{{"k", true}}, ParseTerms([]any{map[any]any{"term": "k"}}))
	require.Equal(t, []Term{{"m", false}}, ParseTerms([]map[string]any{{"term": "m", "enabled": "false"}}))
}

func TestParseQueriesMatchesTheWallhavenGoldenTable(t *testing.T) {
	var golden map[string][]string
	loadGolden(t, "wallhaven_parse_queries.json", &golden)

	// The same inputs tests/golden/capture.py fed _parse_queries.
	inputs := map[string]any{
		"comma":      "a, b ,c",
		"list_dicts": []any{map[string]any{"term": "x", "enabled": true}, map[string]any{"term": "y", "enabled": false}, "z"},
		"empty":      "",
		"empty_list": []any{},
	}
	require.Len(t, golden, len(inputs), "golden table and inputs must cover the same cases")
	for name, in := range inputs {
		want, ok := golden[name]
		require.True(t, ok, "golden case %s", name)
		require.Equal(t, want, enabledQueries(in), "case %s", name)
	}
}

func TestParseQueriesFallsBackToTheDefaultWhenNothingIsEnabled(t *testing.T) {
	require.Equal(t, []string{"landscape"}, ParseQueries(nil, "landscape"))
	require.Equal(t, []string{"landscape"}, ParseQueries("", "landscape"))
	require.Equal(t, []string{"landscape"}, ParseQueries([]any{}, "landscape"))
	require.Equal(t, []string{"landscape"}, ParseQueries([]any{map[string]any{"term": "y", "enabled": false}}, "landscape"))
	require.Equal(t, []string{"x", "z"}, ParseQueries([]any{map[string]any{"term": "x"}, "z"}, "landscape"))
}
