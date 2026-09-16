// Copied from angou (same author) — keep in sync by hand.

package gui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
)

// Font discovery.
//
// Fyne draws its own text and does not consult fontconfig, so a font chooser
// means finding the files ourselves and handing the bytes to the theme. This
// scans the standard font directories once, groups faces into families, and
// keeps only families that have a regular face — a family we cannot render at
// normal weight is not one we can offer.
//
// The scan reads font files and nothing else. It is the only filesystem access
// this package performs.

// fontFamily holds the faces of one family. Missing faces stay nil, and the
// theme falls back to the regular face for them: a family shipping only a
// regular weight renders bold text unbolded, which is better than rendering it
// in an unrelated font.
type fontFamily struct {
	name       string
	regular    string // path
	bold       string
	italic     string
	boldItalic string
}

// defaultFontName is the sentinel for "use the font Fyne ships with".
const defaultFontName = "Fyne default"

var (
	fontsOnce sync.Once
	fontList  []fontFamily
)

func fontDirs() []string {
	dirs := []string{"/usr/share/fonts", "/usr/local/share/fonts"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "share", "fonts"), filepath.Join(home, ".fonts"))
	}
	// macOS.
	dirs = append(dirs, "/System/Library/Fonts", "/Library/Fonts")
	return dirs
}

// families returns the installed families, discovered once.
func families() []fontFamily {
	fontsOnce.Do(func() { fontList = scanFonts() })
	return fontList
}

func scanFonts() []fontFamily {
	byName := map[string]*fontFamily{}

	for _, dir := range fontDirs() {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil //nolint:nilerr // an unreadable font directory is not worth failing over
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".ttf" && ext != ".otf" {
				return nil
			}
			name, style := splitFace(filepath.Base(path))
			if name == "" {
				return nil
			}
			f := byName[name]
			if f == nil {
				f = &fontFamily{name: name}
				byName[name] = f
			}
			switch style {
			case "regular":
				f.regular = path
			case "bold":
				f.bold = path
			case "italic", "oblique":
				f.italic = path
			case "bolditalic", "boldoblique":
				f.boldItalic = path
			}
			return nil
		})
	}

	out := make([]fontFamily, 0, len(byName))
	for _, f := range byName {
		if f.regular != "" {
			out = append(out, *f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// splitFace derives a family name and a style from a font filename.
// "DejaVuSans-BoldOblique.ttf" becomes ("DejaVu Sans", "boldoblique");
// "arial.ttf" becomes ("arial", "regular").
func splitFace(base string) (string, string) {
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	style := "regular"
	if i := strings.LastIndex(stem, "-"); i > 0 {
		style = strings.ToLower(stem[i+1:])
		stem = stem[:i]
	}
	// Variable fonts carry an axis list rather than a weight and cannot be
	// rendered at a fixed style here; skip them rather than offering a family
	// that draws wrong.
	if strings.Contains(strings.ToLower(stem), "[") || strings.Contains(style, "[") {
		return "", ""
	}
	return spaceCamel(stem), style
}

// spaceCamel turns "DejaVuSansMono" into "DejaVu Sans Mono" so the picker reads
// like a font menu rather than a directory listing.
func spaceCamel(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if i == 0 {
			b.WriteRune(r)
			continue
		}
		prevUpper := runes[i-1] >= 'A' && runes[i-1] <= 'Z'
		if r >= 'A' && r <= 'Z' && !prevUpper {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// fontNames lists the choices offered, with the bundled default first.
func fontNames() []string {
	names := []string{defaultFontName}
	for _, f := range families() {
		names = append(names, f.name)
	}
	return names
}

// loadedFont holds the resources for a chosen family. A nil value means the
// bundled default.
type loadedFont struct {
	regular    fyne.Resource
	bold       fyne.Resource
	italic     fyne.Resource
	boldItalic fyne.Resource
}

var (
	fontCacheMu sync.Mutex
	fontCache   = map[string]*loadedFont{}
)

// loadFont reads a family's faces. It returns nil for the default, and nil for
// anything it cannot read — an unreadable font falls back rather than failing,
// because a missing font is not a reason to refuse to draw a window.
func loadFont(name string) *loadedFont {
	if name == "" || name == defaultFontName {
		return nil
	}
	fontCacheMu.Lock()
	defer fontCacheMu.Unlock()
	if f, ok := fontCache[name]; ok {
		return f
	}

	var fam *fontFamily
	for i := range families() {
		if families()[i].name == name {
			fam = &families()[i]
			break
		}
	}
	if fam == nil {
		fontCache[name] = nil
		return nil
	}

	read := func(path string) fyne.Resource {
		if path == "" {
			return nil
		}
		b, err := os.ReadFile(path) //nolint:gosec // a font path this package discovered itself
		if err != nil {
			return nil
		}
		return fyne.NewStaticResource(filepath.Base(path), b)
	}

	lf := &loadedFont{
		regular:    read(fam.regular),
		bold:       read(fam.bold),
		italic:     read(fam.italic),
		boldItalic: read(fam.boldItalic),
	}
	if lf.regular == nil {
		lf = nil
	}
	fontCache[name] = lf
	return lf
}

// face picks the resource for a text style, falling back to regular for any
// face the family does not ship.
func (l *loadedFont) face(s fyne.TextStyle) fyne.Resource {
	if l == nil {
		return nil
	}
	var r fyne.Resource
	switch {
	case s.Bold && s.Italic:
		r = l.boldItalic
	case s.Bold:
		r = l.bold
	case s.Italic:
		r = l.italic
	}
	if r == nil {
		r = l.regular
	}
	return r
}
