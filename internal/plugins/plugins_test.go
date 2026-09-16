package plugins

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/config"
)

func TestRegistryListsPluginsInThePythonManagerOrder(t *testing.T) {
	reg := Registry(Deps{})
	require.Equal(t, []string{"local", "wallhaven", "duckduckgo_images"}, Names(reg))
}

func TestLookupFindsByNameAndReportsUnknownNames(t *testing.T) {
	reg := Registry(Deps{})
	p, ok := Lookup(reg, "wallhaven")
	require.True(t, ok)
	require.Equal(t, "wallhaven", p.Name())
	require.Equal(t, "Download wallpapers from Wallhaven.cc (API v1)", p.Description())

	_, ok = Lookup(reg, "stable_diffusion")
	require.False(t, ok)
}

func TestRegistryFillsNilHTTPClientAndClockSoPluginsNeverDereferenceNil(t *testing.T) {
	reg := Registry(Deps{})
	wh, ok := reg[1].(*wallhaven)
	require.True(t, ok)
	require.NotNil(t, wh.deps.HTTP)
	require.NotNil(t, wh.deps.Now)
	require.False(t, wh.deps.Now().IsZero())
	ddg, ok := reg[2].(*duckduckgo)
	require.True(t, ok)
	require.NotNil(t, ddg.deps.HTTP)
	require.NotNil(t, ddg.deps.Now)
}

// schemaKeys and schemaDefaults project a schema for comparison.
func schemaKeys(s []Field) []string {
	keys := make([]string, len(s))
	for i, f := range s {
		keys[i] = f.Key
	}
	return keys
}

func schemaDefaults(s []Field) map[string]any {
	out := map[string]any{}
	for _, f := range s {
		out[f.Key] = f.Default
	}
	return out
}

func TestLocalSchemaMatchesPythonKeysTypesAndDefaults(t *testing.T) {
	s := newLocal(Deps{}).Schema()
	require.Equal(t, []string{"path", "recursive"}, schemaKeys(s))
	require.Equal(t, map[string]any{"path": nil, "recursive": false}, schemaDefaults(s))

	require.Equal(t, TypeString, s[0].Type)
	require.True(t, s[0].Required)
	require.Equal(t, WidgetDirectoryPath, s[0].Widget)
	require.Equal(t, "Path to file or directory", s[0].Description)
	require.Equal(t, TypeBoolean, s[1].Type)
	require.Equal(t, "Search recursively (if path is directory)", s[1].Description)
}

func TestWallhavenSchemaMatchesPythonKeysTypesAndDefaults(t *testing.T) {
	s := newWallhaven(Deps{}).Schema()
	require.Equal(t, []string{
		"api_key", "query", "sorting", "top_range",
		"category_general", "category_anime", "category_people",
		"purity_sfw", "purity_sketchy", "purity_nsfw",
		"resolutions", "atleast", "ratios", "download_dir", "interval", "limit", "max_files",
	}, schemaKeys(s))

	home := config.HomeDir()
	require.Equal(t, map[string]any{
		"api_key":          "",
		"query":            []Term{{Term: "landscape", Enabled: true}},
		"sorting":          "relevance",
		"top_range":        "1M",
		"category_general": true,
		"category_anime":   true,
		"category_people":  true,
		"purity_sfw":       true,
		"purity_sketchy":   false,
		"purity_nsfw":      false,
		"resolutions":      "",
		"atleast":          "2560x1440",
		"ratios":           "16x9",
		"download_dir":     filepath.Join(home, "Pictures", "Wallpapers", "Wallhaven"),
		"interval":         "Daily",
		"limit":            10,
		"max_files":        100,
	}, schemaDefaults(s))

	byKey := map[string]Field{}
	for _, f := range s {
		byKey[f.Key] = f
	}
	require.Equal(t, TypeStringList, byKey["query"].Type)
	require.Equal(t, []string{"landscape", "cyberpunk", "pixel art", "4k", "toplist"}, byKey["query"].Suggestions)
	require.Equal(t, []string{"relevance", "random", "date_added", "views", "favorites", "toplist"}, byKey["sorting"].Enum)
	require.Equal(t, []string{"1d", "3d", "1w", "1M", "3M", "6M", "1y"}, byKey["top_range"].Enum)
	for _, k := range []string{"category_general", "category_anime", "category_people"} {
		require.Equal(t, TypeBoolean, byKey[k].Type)
		require.Equal(t, "Categories", byKey[k].Group)
	}
	for _, k := range []string{"purity_sfw", "purity_sketchy", "purity_nsfw"} {
		require.Equal(t, TypeBoolean, byKey[k].Type)
		require.Equal(t, "Purity", byKey[k].Group)
	}
	require.Equal(t, "General", byKey["category_general"].Description)
	require.Equal(t, "NSFW", byKey["purity_nsfw"].Description)
	require.Equal(t, []string{"1920x1080", "2560x1440", "3840x2160"}, byKey["resolutions"].Suggestions)
	require.Equal(t, []string{"1920x1080", "2560x1440", "3840x2160"}, byKey["atleast"].Suggestions)
	require.Equal(t, []string{"16x9", "21x9", "16x10", "portrait"}, byKey["ratios"].Suggestions)
	require.Equal(t, WidgetDirectoryPath, byKey["download_dir"].Widget)
	require.Equal(t, []string{"Hourly", "Daily", "Weekly"}, byKey["interval"].Enum)
	require.Equal(t, TypeInteger, byKey["limit"].Type)
	require.Equal(t, "Max Downloads per run", byKey["limit"].Description)
	require.Equal(t, TypeInteger, byKey["max_files"].Type)
	require.Equal(t, "Retention Limit (Max Files)", byKey["max_files"].Description)
	require.Equal(t, "API Key (Optional, required for NSFW)", byKey["api_key"].Description)
	for _, f := range s {
		require.False(t, f.Required, "wallhaven has no required fields: %s", f.Key)
	}
}

func TestDuckDuckGoSchemaMatchesPythonKeysTypesAndDefaults(t *testing.T) {
	s := newDuckDuckGo(Deps{}).Schema()
	require.Equal(t, []string{"query", "download_dir", "interval", "limit", "max_files"}, schemaKeys(s))

	home := config.HomeDir()
	require.Equal(t, map[string]any{
		"query":        []Term{{Term: "4k nature wallpapers", Enabled: true}},
		"download_dir": filepath.Join(home, "Pictures", "Wallpapers", "DuckDuckGo"),
		"interval":     "Daily",
		"limit":        10,
		"max_files":    50,
	}, schemaDefaults(s))

	require.Equal(t, TypeStringList, s[0].Type)
	require.Equal(t, "Search Terms", s[0].Description)
	require.Equal(t, []string{
		"4k nature wallpapers",
		"4k space wallpapers",
		"site:reddit.com r/SpacePorn",
		"site:reddit.com r/EarthPorn",
		"site:reddit.com r/SkyPorn",
		"site:reddit.com r/Animals",
		"site:reddit.com r/Wallpapers",
		"4k cityscapes",
		"4k abstract wallpapers",
		"4k landscape wallpapers",
	}, s[0].Suggestions)
	require.Equal(t, "Download Path", s[1].Description)
	require.Equal(t, WidgetDirectoryPath, s[1].Widget)
	require.Equal(t, []string{"Hourly", "Daily", "Weekly"}, s[2].Enum)
	require.Equal(t, "Check Interval", s[2].Description)
	require.Equal(t, TypeInteger, s[3].Type)
	require.Equal(t, "Max Downloads (HQ)", s[3].Description)
	require.Equal(t, TypeInteger, s[4].Type)
	require.Equal(t, "Retention Limit", s[4].Description)
}
