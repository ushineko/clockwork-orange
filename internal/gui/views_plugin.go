package gui

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ushineko/clockwork-orange/internal/core"
	"github.com/ushineko/clockwork-orange/internal/plugins"
)

// --- Plugin sections (R7.5) ---------------------------------------------------

/*
pluginForm is one plugin's configuration as widgets, generated from its
schema the way plugins_tab.py generated Qt widgets: string → Entry, or Select
for an enum, or an editable Select with suggestions; boolean → Check;
integer → numeric Entry 0–10000; string_list → searchTerms; file_path and
directory_path → an Entry with Browse (and Open for a directory). Consecutive
fields sharing a group go on one row under the group's label.

Every widget writes into the block on change, and the block goes into the
document (which schedules the save). Keys the schema does not know stay in
the block untouched.
*/
type pluginForm struct {
	name    string
	schema  []plugins.Field
	block   map[string]any
	enabled *widget.Check
	fields  map[string]fieldWidget
	body    fyne.CanvasObject
	// onChange is called after every edit; the section wires it to the
	// document. Nil in tests that only inspect the widgets.
	onChange func(block map[string]any)
	// setting suppresses onChange while the widgets are being filled.
	setting bool
}

// fieldWidget reads one field's current value back out of its widget.
type fieldWidget interface {
	value() any
}

type entryField struct{ e *widget.Entry }

func (f entryField) value() any { return f.e.Text }

type intField struct{ e *widget.Entry }

func (f intField) value() any {
	n, err := strconv.Atoi(strings.TrimSpace(f.e.Text))
	if err != nil {
		return 0
	}
	return n
}

type selectField struct{ s *widget.Select }

func (f selectField) value() any { return f.s.Selected }

type selectEntryField struct{ s *widget.SelectEntry }

func (f selectEntryField) value() any { return f.s.Text }

type checkField struct{ c *widget.Check }

func (f checkField) value() any { return f.c.Checked }

// newPluginForm builds the form over a copy of the plugin's block.
func (u *ui) newPluginForm(info core.PluginInfo, block map[string]any, onChange func(map[string]any)) *pluginForm {
	f := &pluginForm{name: info.Name, schema: info.Schema, block: map[string]any{}, fields: map[string]fieldWidget{}, onChange: onChange}
	for k, v := range block {
		f.block[k] = v
	}
	f.setting = true
	defer func() { f.setting = false }()

	f.enabled = widget.NewCheck("Enable this plugin", func(b bool) { f.changed("enabled", b) })
	f.enabled.SetChecked(truthy(block["enabled"]))

	rows := []fyne.CanvasObject{f.enabled}
	items := []*widget.FormItem{}
	flushItems := func() {
		if len(items) > 0 {
			rows = append(rows, widget.NewForm(items...))
			items = nil
		}
	}
	for i := 0; i < len(info.Schema); {
		field := info.Schema[i]
		if field.Group == "" {
			items = append(items, widget.NewFormItem(field.Description, u.fieldWidget(f, field, block)))
			i++
			continue
		}
		// Consecutive fields of one group share a row (plugins_tab.py
		// _add_grouped_config_fields).
		flushItems()
		row := container.NewHBox(widget.NewLabelWithStyle(field.Group+":", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		for i < len(info.Schema) && info.Schema[i].Group == field.Group {
			g := info.Schema[i]
			if g.Type == plugins.TypeBoolean {
				c := widget.NewCheck(g.Description, func(b bool) { f.changed(g.Key, b) })
				c.SetChecked(truthy(valueOr(block, g.Key, g.Default)))
				f.fields[g.Key] = checkField{c}
				row.Add(c)
			} else {
				row.Add(widget.NewLabel(g.Description))
				row.Add(u.fieldWidget(f, g, block))
			}
			i++
		}
		rows = append(rows, row)
	}
	flushItems()
	f.body = container.NewVBox(rows...)
	return f
}

// fieldWidget builds one field's editor and registers its reader.
func (u *ui) fieldWidget(f *pluginForm, field plugins.Field, block map[string]any) fyne.CanvasObject {
	current := valueOr(block, field.Key, field.Default)
	switch field.Type {
	case plugins.TypeBoolean:
		c := widget.NewCheck("", func(b bool) { f.changed(field.Key, b) })
		c.SetChecked(truthy(current))
		f.fields[field.Key] = checkField{c}
		return c
	case plugins.TypeInteger:
		e := widget.NewEntry()
		e.Validator = intRange(0, 10000)
		e.SetText(fmt.Sprintf("%v", intOf(current)))
		e.OnChanged = func(s string) {
			if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n >= 0 && n <= 10000 {
				f.changed(field.Key, n)
			}
		}
		f.fields[field.Key] = intField{e}
		return fixedWidth(e, 120)
	case plugins.TypeStringList:
		st := newSearchTerms(field.Suggestions, func(v []map[string]any) { f.changed(field.Key, v) })
		st.set(current)
		f.fields[field.Key] = st
		return st.body
	case plugins.TypeString:
	}
	// string
	text := stringOf(current)
	switch {
	case len(field.Enum) > 0:
		s := widget.NewSelect(field.Enum, func(v string) { f.changed(field.Key, v) })
		s.SetSelected(text)
		f.fields[field.Key] = selectField{s}
		return s
	case len(field.Suggestions) > 0:
		s := widget.NewSelectEntry(field.Suggestions)
		s.SetText(text)
		s.OnChanged = func(v string) { f.changed(field.Key, v) }
		f.fields[field.Key] = selectEntryField{s}
		return s
	}
	e := widget.NewEntry()
	if field.Key == "api_key" {
		e.Password = true
	}
	e.SetText(text)
	e.OnChanged = func(v string) { f.changed(field.Key, v) }
	f.fields[field.Key] = entryField{e}
	switch field.Widget {
	case plugins.WidgetFilePath:
		return u.withBrowse(e, false)
	case plugins.WidgetDirectoryPath:
		open := widget.NewButtonWithIcon("Open", theme.FolderIcon(), func() { u.openPath(e.Text) })
		return container.NewBorder(nil, nil, nil, container.NewHBox(u.browseButton(e, true), open), e)
	case plugins.WidgetNone:
	}
	return e
}

// changed records one field's new value and reports the block.
func (f *pluginForm) changed(key string, v any) {
	if f.setting {
		return
	}
	f.block[key] = v
	if f.onChange != nil {
		f.onChange(f.values())
	}
}

// values is the block as the widgets now have it: every schema field plus
// enabled, over the keys the form does not know.
func (f *pluginForm) values() map[string]any {
	out := map[string]any{}
	for k, v := range f.block {
		out[k] = v
	}
	out["enabled"] = f.enabled.Checked
	for key, w := range f.fields {
		out[key] = w.value()
	}
	return out
}

// valueOr is the block's value for key, else the schema default.
func valueOr(block map[string]any, key string, def any) any {
	if v, ok := block[key]; ok && v != nil {
		return v
	}
	return def
}

func truthy(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return strings.EqualFold(b, "true")
	case int:
		return b != 0
	}
	return false
}

func intOf(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	}
	return 0
}

func stringOf(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// --- the search-terms widget -------------------------------------------------

/*
searchTerms is the string_list editor (plugins_tab.py SearchTermsWidget): a
checkable list of terms, an entry with suggestions, Add, and a remove button
per row. Its value is a list of {term, enabled}; it accepts the legacy
comma-separated string and a list of bare strings, and writes the list form.
*/
type searchTerms struct {
	terms    []searchTerm
	list     *fyne.Container
	entry    *widget.SelectEntry
	body     fyne.CanvasObject
	onChange func([]map[string]any)
	setting  bool
}

type searchTerm struct {
	term    string
	enabled bool
}

func newSearchTerms(suggestions []string, onChange func([]map[string]any)) *searchTerms {
	st := &searchTerms{onChange: onChange, list: container.NewVBox()}
	st.entry = widget.NewSelectEntry(suggestions)
	st.entry.SetPlaceHolder("Enter a search term…")
	add := widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), st.addFromEntry)
	st.entry.OnSubmitted = func(string) { st.addFromEntry() }
	st.body = container.NewVBox(st.list, container.NewBorder(nil, nil, nil, add, st.entry))
	return st
}

// parseTerms accepts every shape the config has held.
func parseTerms(v any) []searchTerm {
	var out []searchTerm
	switch data := v.(type) {
	case string:
		for _, t := range strings.Split(data, ",") {
			if t = strings.TrimSpace(t); t != "" {
				out = append(out, searchTerm{term: t, enabled: true})
			}
		}
	case []any:
		for _, item := range data {
			switch it := item.(type) {
			case string:
				if it != "" {
					out = append(out, searchTerm{term: it, enabled: true})
				}
			case map[string]any:
				term := stringOf(it["term"])
				if term == "" {
					continue
				}
				enabled := true
				if e, ok := it["enabled"]; ok {
					enabled = truthy(e)
				}
				out = append(out, searchTerm{term: term, enabled: enabled})
			}
		}
	case []map[string]any:
		for _, it := range data {
			term := stringOf(it["term"])
			if term == "" {
				continue
			}
			enabled := true
			if e, ok := it["enabled"]; ok {
				enabled = truthy(e)
			}
			out = append(out, searchTerm{term: term, enabled: enabled})
		}
	case []string:
		for _, t := range data {
			if t != "" {
				out = append(out, searchTerm{term: t, enabled: true})
			}
		}
	}
	return out
}

// set replaces the terms from a config value.
func (st *searchTerms) set(v any) {
	st.setting = true
	st.terms = parseTerms(v)
	st.rebuild()
	st.setting = false
}

// value is the list form the config stores.
func (st *searchTerms) value() any {
	out := make([]map[string]any, 0, len(st.terms))
	for _, t := range st.terms {
		out = append(out, map[string]any{"term": t.term, "enabled": t.enabled})
	}
	return out
}

// enabledTerms is the checked terms, for the run dialog's checklist.
func (st *searchTerms) enabledTerms() []string {
	var out []string
	for _, t := range st.terms {
		if t.enabled {
			out = append(out, t.term)
		}
	}
	return out
}

func (st *searchTerms) addFromEntry() {
	text := strings.TrimSpace(st.entry.Text)
	if text == "" {
		return
	}
	st.terms = append(st.terms, searchTerm{term: text, enabled: true})
	st.entry.SetText("")
	st.rebuild()
	st.report()
}

func (st *searchTerms) remove(i int) {
	if i < 0 || i >= len(st.terms) {
		return
	}
	st.terms = append(st.terms[:i], st.terms[i+1:]...)
	st.rebuild()
	st.report()
}

func (st *searchTerms) report() {
	if st.setting || st.onChange == nil {
		return
	}
	v, _ := st.value().([]map[string]any)
	st.onChange(v)
}

// rebuild redraws the rows from the terms.
func (st *searchTerms) rebuild() {
	st.list.Objects = nil
	for i := range st.terms {
		idx := i
		c := widget.NewCheck(st.terms[i].term, func(b bool) {
			if idx < len(st.terms) {
				st.terms[idx].enabled = b
				st.report()
			}
		})
		c.SetChecked(st.terms[i].enabled)
		rm := widget.NewButtonWithIcon("", theme.ContentRemoveIcon(), func() { st.remove(idx) })
		rm.Importance = widget.LowImportance
		st.list.Add(container.NewBorder(nil, nil, nil, rm, c))
	}
	st.list.Refresh()
}

// --- the section -------------------------------------------------------------

// pluginInfo finds a plugin's registry entry by name.
func (u *ui) pluginInfo(name string) (core.PluginInfo, bool) {
	if u.plugins == nil {
		u.plugins = core.AvailablePlugins()
	}
	for _, p := range u.plugins {
		if p.Name == name {
			return p, true
		}
	}
	return core.PluginInfo{}, false
}

/*
buildPlugin is one plugin's section: its description, the generated form, the
toolbar (Download Now, Reset & Run, Apply Blacklist) and the image review of
its directory (R7.5, R7.6).
*/
func (u *ui) buildPlugin(name string) fyne.CanvasObject {
	info, ok := u.pluginInfo(name)
	if !ok {
		return heading(pluginTitle(name), "This build has no plugin of that name.")
	}
	form := u.newPluginForm(info, u.doc.Plugins[name], func(block map[string]any) {
		u.doc.SetPlugin(name, block)
		u.scheduleSave()
		u.redrawStatus()
	})

	runnable := name != "local"
	download := widget.NewButtonWithIcon("Download now", theme.DownloadIcon(), func() {
		u.openRunDialog(name, "Download now", form, false)
	})
	download.Importance = widget.HighImportance
	reset := widget.NewButtonWithIcon("Reset & run", theme.ViewRefreshIcon(), func() {
		u.confirmDestructive("Delete all downloaded files and run fresh?",
			"Every file in "+pluginDir(info, form.values())+" is deleted, then the plugin downloads "+
				"again. The history and the blacklist are not touched, so images already seen are "+
				"not fetched a second time.", "Delete and run", func() {
				u.openRunDialog(name, "Reset & run", form, true)
			})
	})
	if !runnable {
		download.Disable()
		reset.Disable()
	}
	u.gate(download, reset)

	rv := u.reviewFor(name, info, form.values())
	apply := widget.NewButtonWithIcon(fmt.Sprintf("Apply blacklist (%d)", rv.markedCount()), theme.DeleteIcon(), func() {
		u.applyBlacklist(name, rv)
	})
	apply.Importance = widget.DangerImportance
	if rv.markedCount() == 0 || u.working() {
		apply.Disable()
	}
	rv.applyBtn = apply
	rescan := widget.NewButtonWithIcon("Rescan", theme.ViewRefreshIcon(), func() {
		rv.scan()
		rv.draw(u)
	})

	// Two tabs (R7.18): the form and its actions, and the review. In one
	// column the review sat below the fold of every plugin with more than a
	// few settings, and the keys it listens for went to a pane nobody could
	// see. The selected tab survives the rebuilds every operation causes.
	configTab := container.NewVBox(
		card("Configuration", form.body),
		card("Actions", container.NewHBox(download, reset)),
	)
	// Plain words, no arrow glyphs: the arrows come from a fallback font and
	// Fyne's shaper drew the run boundary after them as a missing glyph.
	hint := dim("Arrow keys: previous/next  ·  Space: mark/unmark")
	toolbar := container.NewBorder(nil, nil,
		container.NewHBox(apply, rescan), hint,
		dim(fmt.Sprintf("  %d image(s) in %s", len(rv.images), rv.dir)))
	reviewTab := container.NewBorder(toolbar, nil, nil, nil, rv.widget(u))
	// Only the selected tab carries its real content; the other holds an
	// empty box. AppTabs sizes itself to its tallest item, so with both
	// present a plugin with a long form (Wallhaven) laid its Review tab out
	// in a pane taller than the window and the image sat off-centre. A tab
	// change rebuilds the section, which is how every other state change
	// here is drawn too.
	var configContent, reviewContent fyne.CanvasObject = container.NewWithoutLayout(), container.NewWithoutLayout()
	if u.pluginTab == 1 {
		reviewContent = reviewTab
	} else {
		configContent = configTab
	}
	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Configuration", theme.SettingsIcon(), configContent),
		container.NewTabItemWithIcon("Review", theme.FileImageIcon(), reviewContent),
	)
	tabs.SelectIndex(u.pluginTab)
	tabs.OnSelected = func(*container.TabItem) {
		if tabs.SelectedIndex() == u.pluginTab {
			return
		}
		u.pluginTab = tabs.SelectedIndex()
		u.refresh()
	}

	// Border, not VBox: the content pane is a Scroll, which sizes its content
	// to at least the viewport, so the tabs (and the preview inside them)
	// take the section's full height instead of their minimum.
	return container.NewBorder(heading(pluginTitle(name), info.Description), nil, nil, nil, tabs)
}

// pluginDir is the directory a plugin's review scans: `path` for local,
// `download_dir` for the others, falling back to the schema default.
func pluginDir(info core.PluginInfo, block map[string]any) string {
	key := "download_dir"
	if info.Name == "local" {
		key = "path"
	}
	if s := stringOf(block[key]); s != "" {
		return s
	}
	for _, f := range info.Schema {
		if f.Key == key {
			return stringOf(f.Default)
		}
	}
	return ""
}
