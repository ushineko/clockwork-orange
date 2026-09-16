package gui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/core"
	"github.com/ushineko/clockwork-orange/internal/plugins"
)

// schema is a plugin schema exercising every field shape the generator must
// handle (R7.5).
func testSchema() core.PluginInfo {
	return core.PluginInfo{Name: "probe", Description: "a probe", Schema: []plugins.Field{
		{Key: "plain", Type: plugins.TypeString, Description: "Plain", Default: "d"},
		{Key: "choice", Type: plugins.TypeString, Description: "Choice", Enum: []string{"a", "b"}, Default: "a"},
		{Key: "hint", Type: plugins.TypeString, Description: "Hint", Suggestions: []string{"x", "y"}, Default: "x"},
		{Key: "file", Type: plugins.TypeString, Description: "File", Widget: plugins.WidgetFilePath},
		{Key: "dir", Type: plugins.TypeString, Description: "Dir", Widget: plugins.WidgetDirectoryPath, Default: "~/Pictures"},
		{Key: "flag", Type: plugins.TypeBoolean, Description: "Flag", Default: false},
		{Key: "count", Type: plugins.TypeInteger, Description: "Count", Default: 10},
		{Key: "terms", Type: plugins.TypeStringList, Description: "Terms", Default: "landscape", Suggestions: []string{"forest"}},
		{Key: "g1", Type: plugins.TypeBoolean, Description: "General", Group: "Categories", Default: true},
		{Key: "g2", Type: plugins.TypeBoolean, Description: "Anime", Group: "Categories", Default: false},
		{Key: "g3", Type: plugins.TypeBoolean, Description: "People", Group: "Categories", Default: false},
		{Key: "after", Type: plugins.TypeString, Description: "After", Default: ""},
	}}
}

// Each field type gets the editor plugins_tab.py gave it: an enum is a Select
// (no free text), suggestions an editable SelectEntry, boolean a Check,
// integer a numeric Entry, string_list the terms widget, and the two path
// widgets keep an Entry underneath their Browse buttons.
func TestPluginFormGeneratesTheRightEditorPerFieldType(t *testing.T) {
	u, _, _ := testUI(t)
	f := u.newPluginForm(testSchema(), map[string]any{"enabled": true, "choice": "b", "count": 7}, nil)

	require.True(t, f.enabled.Checked)
	require.IsType(t, entryField{}, f.fields["plain"])
	require.Equal(t, "d", f.fields["plain"].value(), "the schema default fills an absent key")
	require.IsType(t, selectField{}, f.fields["choice"])
	require.Equal(t, "b", f.fields["choice"].value(), "the block's value wins over the default")
	require.IsType(t, selectEntryField{}, f.fields["hint"])
	require.Equal(t, "x", f.fields["hint"].value())
	require.IsType(t, entryField{}, f.fields["file"])
	require.IsType(t, entryField{}, f.fields["dir"])
	require.Equal(t, "~/Pictures", f.fields["dir"].value())
	require.IsType(t, checkField{}, f.fields["flag"])
	require.Equal(t, false, f.fields["flag"].value())
	require.IsType(t, intField{}, f.fields["count"])
	require.Equal(t, 7, f.fields["count"].value())
	require.IsType(t, &searchTerms{}, f.fields["terms"])
	for _, k := range []string{"g1", "g2", "g3"} {
		require.IsType(t, checkField{}, f.fields[k], k)
	}
	require.Equal(t, true, f.fields["g1"].value())
	require.Len(t, f.fields, 12)
}

// Consecutive fields of one group share a row: the form comes out as a Form
// (plain, choice, hint, file, dir, flag, count, terms), one HBox row for the
// three Categories checks, and a Form for the field after them.
func TestPluginFormPacksAGroupOntoOneRow(t *testing.T) {
	u, _, _ := testUI(t)
	f := u.newPluginForm(testSchema(), nil, nil)
	body := f.body.(*fyne.Container)
	require.Len(t, body.Objects, 4, "enable check, the form before the group, the group row, the form after")
	require.IsType(t, &widget.Check{}, body.Objects[0])
	require.IsType(t, &widget.Form{}, body.Objects[1])
	require.Len(t, body.Objects[1].(*widget.Form).Items, 8)
	row := body.Objects[2].(*fyne.Container)
	require.Len(t, row.Objects, 4, "the group label and its three checks share one row")
	require.Equal(t, "Categories:", row.Objects[0].(*widget.Label).Text)
	require.IsType(t, &widget.Form{}, body.Objects[3])
	require.Len(t, body.Objects[3].(*widget.Form).Items, 1)
}

// An edit writes into the block and reports every value, including the keys
// the schema does not know, so the document keeps them (D6).
func TestPluginFormReportsTheWholeBlockOnEveryEdit(t *testing.T) {
	u, _, _ := testUI(t)
	var got map[string]any
	f := u.newPluginForm(testSchema(), map[string]any{"enabled": false, "mystery": "kept"}, func(b map[string]any) { got = b })
	require.Nil(t, got, "filling the widgets must not count as an edit")

	f.enabled.SetChecked(true)
	require.NotNil(t, got)
	require.Equal(t, true, got["enabled"])
	require.Equal(t, "kept", got["mystery"])
	require.Equal(t, 10, got["count"], "the default is written out so the file says what the plugin will use")

	f.fields["choice"].(selectField).s.SetSelected("b")
	require.Equal(t, "b", got["choice"])
	f.fields["count"].(intField).e.SetText("20000")
	require.Equal(t, 10, got["count"], "a value over 10000 is refused")
	f.fields["count"].(intField).e.SetText("25")
	require.Equal(t, 25, got["count"])
	f.fields["g2"].(checkField).c.SetChecked(true)
	require.Equal(t, true, got["g2"])
}

// The terms widget accepts the three shapes the config has held and always
// writes the list form; the legacy comma string round-trips into it.
func TestSearchTermsRoundTripsLegacyStringsAndLists(t *testing.T) {
	var got []map[string]any
	st := newSearchTerms([]string{"forest"}, func(v []map[string]any) { got = v })

	st.set("landscape, mountains ,, sea")
	require.Equal(t, []searchTerm{{"landscape", true}, {"mountains", true}, {"sea", true}}, st.terms)
	require.Nil(t, got, "set is not an edit")
	require.Equal(t, []map[string]any{
		{"term": "landscape", "enabled": true}, {"term": "mountains", "enabled": true}, {"term": "sea", "enabled": true},
	}, st.value())

	st.set([]any{map[string]any{"term": "a", "enabled": false}, "b", map[string]any{"term": ""}})
	require.Equal(t, []searchTerm{{"a", false}, {"b", true}}, st.terms)
	require.Equal(t, []string{"b"}, st.enabledTerms())

	st.entry.SetText("  c ")
	st.addFromEntry()
	require.Equal(t, []searchTerm{{"a", false}, {"b", true}, {"c", true}}, st.terms)
	require.Len(t, got, 3, "Add reports the new list")
	require.Equal(t, "", st.entry.Text)

	st.remove(1)
	require.Equal(t, []searchTerm{{"a", false}, {"c", true}}, st.terms)
	require.Len(t, got, 2)

	// Ticking a row's box reports too.
	findCheck(st.list.Objects[1]).SetChecked(false)
	require.Equal(t, []searchTerm{{"a", false}, {"c", false}}, st.terms)
	require.Equal(t, false, got[1]["enabled"])
}

// Download Now is disabled for the local plugin, which has nothing to
// download, and enabled for the others.
func TestLocalPluginHasNoDownloadButton(t *testing.T) {
	u, _, _ := testUI(t)
	local := u.buildPlugin("local")
	wh := u.buildPlugin("wallhaven")
	require.True(t, findButton(local, "Download now").Disabled())
	require.False(t, findButton(wh, "Download now").Disabled())
	require.Nil(t, findButton(wh, "Apply blacklist (0)"), "the Review tab's content is built only when it is selected")
	u.pluginTab = 1
	wh = u.buildPlugin("wallhaven")
	require.Nil(t, findButton(wh, "Download now"), "and the Configuration tab's only when it is")
	require.True(t, findButton(wh, "Apply blacklist (0)").Disabled(), "nothing marked, nothing to apply")
}
