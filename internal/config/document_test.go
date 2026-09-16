package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseThenMarshalRoundTripsALoadedDocumentUnchanged(t *testing.T) {
	// A round trip that drifts would rewrite users' files on every save.
	src := goldenPath(t, "config", "post_migration.yml")
	b, err := os.ReadFile(src)
	require.NoError(t, err)

	d1, err := Parse(b)
	require.NoError(t, err)
	b2, err := Marshal(d1)
	require.NoError(t, err)
	d2, err := Parse(b2)
	require.NoError(t, err)
	require.Equal(t, d1, d2)

	b3, err := Marshal(d2)
	require.NoError(t, err)
	require.Equal(t, string(b2), string(b3), "second marshal must be byte-identical")
	require.Equal(t, parseYAMLMap(t, b), parseYAMLMap(t, b2))
}

func TestParsePreservesUnknownPluginBlocksAndTopLevelKeys(t *testing.T) {
	// D6/DV7: the stable_diffusion block and unknown keys must survive until
	// the plugin returns; 2.9.x clean_config deleted them.
	d, err := Parse([]byte(`
default_wait: 120
unknown_top_level:
  keep: me
plugins:
  stable_diffusion:
    enabled: false
    steps: 30
    prompt:
      - term: a castle
        enabled: true
  local:
    enabled: true
`))
	require.NoError(t, err)
	require.Equal(t, 120, d.DefaultWait)
	require.Equal(t, map[string]any{"keep": "me"}, d.Extra["unknown_top_level"])
	require.Contains(t, d.Plugins, "stable_diffusion")
	require.Equal(t, 30, d.Plugins["stable_diffusion"]["steps"])
	require.Len(t, d.Plugins["stable_diffusion"]["prompt"], 1)

	out, err := Marshal(d)
	require.NoError(t, err)
	require.Contains(t, string(out), "stable_diffusion:")
	require.Contains(t, string(out), "unknown_top_level:")
}

func TestMarshalWritesKeysSortedWithTwoSpaceIndent(t *testing.T) {
	// Python wrote sort_keys=True; a different order shows up as noise in
	// every dotfiles diff.
	d := Document{
		DualWallpapers: true,
		DefaultWait:    120,
		Extra:          map[string]any{"zeta": 1, "alpha": "x"},
		Plugins: map[string]map[string]any{
			"local":     {"path": "~/Pictures", "enabled": true},
			"wallhaven": {"enabled": false, "api_key": ""},
		},
	}
	out, err := Marshal(d)
	require.NoError(t, err)
	want := `alpha: x
default_wait: 120
dual_wallpapers: true
plugins:
  local:
    enabled: true
    path: ~/Pictures
  wallhaven:
    api_key: ""
    enabled: false
zeta: 1
`
	require.Equal(t, want, string(out))
}

func TestMarshalSortsKeysBytewiseNotNaturally(t *testing.T) {
	// yaml.v3's own map ordering puts "item9" before "item10"; Python did not.
	out, err := Marshal(Document{Extra: map[string]any{"item10": 1, "item9": 2}})
	require.NoError(t, err)
	require.Equal(t, "item10: 1\nitem9: 2\n", string(out))
}

func TestMarshalOmitsAutoUpdateLogsWhenFalseEvenIfTheFileSpelledItOut(t *testing.T) {
	d, err := Parse([]byte("auto_update_logs: false\n"))
	require.NoError(t, err)
	out, err := Marshal(d)
	require.NoError(t, err)
	require.Equal(t, "{}\n", string(out))

	d.AutoUpdateLogs = true
	out, err = Marshal(d)
	require.NoError(t, err)
	require.Equal(t, "auto_update_logs: true\n", string(out))
}

func TestMarshalKeepsExplicitZeroValuesTheOldGUIWrote(t *testing.T) {
	// The 2.9.x GUI wrote `desktop: false` etc. Dropping them would be a
	// semantic no-op but rewrites a file the user did not touch.
	d, err := Parse([]byte("debug: false\ndefault_url: ''\ndefault_wait: 0\ndesktop: false\nplugins: {}\n"))
	require.NoError(t, err)
	out, err := Marshal(d)
	require.NoError(t, err)
	require.Equal(t, "debug: false\ndefault_url: \"\"\ndefault_wait: 0\ndesktop: false\nplugins: {}\n", string(out))
}

func TestMarshalOmitsKeysNothingSet(t *testing.T) {
	// Python only emitted keys something had assigned; a Go-built document
	// must not spray every schema key with zero values into the file.
	out, err := Marshal(Document{DefaultWait: 120})
	require.NoError(t, err)
	require.Equal(t, "default_wait: 120\n", string(out))
}

func TestDefaultsMatchTheGUIDefaults(t *testing.T) {
	d := Defaults()
	require.Equal(t, 300, d.DefaultWait)
	require.Equal(t, "Monospace", d.ConsoleFontFamily)
	require.Equal(t, 10, d.ConsoleFontSize)
	require.Equal(t, 800, d.WindowWidth)
	require.Equal(t, 600, d.WindowHeight)
	require.Equal(t, ".jpg,.jpeg,.png,.bmp,.gif,.tiff,.webp,.svg", d.ImageExtensions)
	require.Equal(t, 10, d.RestartDelay)
	require.Equal(t, 5, d.LogsRefreshInterval)
	require.False(t, d.AutoUpdateLogs)
	require.False(t, d.Desktop)
	require.NotNil(t, d.Plugins)
	require.NotNil(t, d.Extra)
	require.Equal(t, 900, DefaultWaitService)
}

func TestParseAcceptsNumericStringsForIntegerKeysLikePythonDid(t *testing.T) {
	d, err := Parse([]byte("default_wait: '120'\n"))
	require.NoError(t, err)
	require.Equal(t, 120, d.DefaultWait)
}

func TestParseRejectsValuesOfTheWrongType(t *testing.T) {
	_, err := Parse([]byte("desktop: maybe\n"))
	require.ErrorContains(t, err, "desktop")

	_, err = Parse([]byte("default_wait: soon\n"))
	require.ErrorContains(t, err, "default_wait")

	_, err = Parse([]byte("plugins: [local]\n"))
	require.ErrorContains(t, err, "plugins")

	_, err = Parse([]byte("plugins:\n  local: yes\n"))
	require.ErrorContains(t, err, "plugins.local")
}

func TestParseTreatsAnEmptyPluginBodyAsAnEmptyBlock(t *testing.T) {
	d, err := Parse([]byte("plugins:\n  local:\n"))
	require.NoError(t, err)
	require.Equal(t, map[string]any{}, d.Plugins["local"])
}

func TestParseOfEmptyInputYieldsAnEmptyDocumentNotAnError(t *testing.T) {
	d, err := Parse(nil)
	require.NoError(t, err)
	require.Empty(t, d.Plugins)
	require.Empty(t, d.Extra)
}

func TestPluginEnabledUsesPythonTruthiness(t *testing.T) {
	d, err := Parse([]byte(`
plugins:
  a: {enabled: true}
  b: {enabled: 1}
  c: {enabled: "yes"}
  d: {enabled: false}
  e: {enabled: 0}
  f: {enabled: ""}
  g: {}
`))
	require.NoError(t, err)
	for _, name := range []string{"a", "b", "c"} {
		require.True(t, d.PluginEnabled(name), name)
	}
	for _, name := range []string{"d", "e", "f", "g", "missing"} {
		require.False(t, d.PluginEnabled(name), name)
	}
	require.Equal(t, []string{"a", "b", "c"}, d.EnabledPlugins())
}

func TestSetPluginWorksOnAZeroDocument(t *testing.T) {
	var d Document
	d.SetPlugin("local", map[string]any{"enabled": true})
	require.True(t, d.PluginEnabled("local"))
	require.Equal(t, []string{"local"}, d.EnabledPlugins())
}
