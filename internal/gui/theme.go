// Copied from angou (same author) — keep in sync by hand; the mono face is this project's.

package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// A KDE color scheme, transcribed from the .colors files shipped by
// plasma-workspace. The field names follow KDE's own vocabulary rather than
// Fyne's so the transcription can be checked against the source scheme without
// a translation step; palette.fyne() does the translation.
//
// Only the roles this window actually paints are carried. A scheme has more
// (Complementary, Header, and a per-role Inactive set) and adding them here
// without a widget that uses them would be noise.
type palette struct {
	name string
	dark bool

	windowBG    color.Color // [Colors:Window]  BackgroundNormal
	windowFG    color.Color // [Colors:Window]  ForegroundNormal
	viewBG      color.Color // [Colors:View]    BackgroundNormal
	viewAltBG   color.Color // [Colors:View]    BackgroundAlternate
	viewFG      color.Color // [Colors:View]    ForegroundNormal
	buttonBG    color.Color // [Colors:Button]  BackgroundNormal
	buttonFG    color.Color // [Colors:Button]  ForegroundNormal
	selectionBG color.Color // [Colors:Selection] BackgroundNormal
	selectionFG color.Color // [Colors:Selection] ForegroundNormal
	tooltipBG   color.Color // [Colors:Tooltip] BackgroundNormal
	inactiveFG  color.Color // ForegroundInactive — disabled text and placeholders
	negative    color.Color // ForegroundNegative
	positive    color.Color // ForegroundPositive
	neutral     color.Color // ForegroundNeutral
	link        color.Color // ForegroundLink
	focus       color.Color // DecorationFocus
	hover       color.Color // DecorationHover
	separator   color.Color // no KDE equivalent: derived, see below
}

func rgb(r, g, b uint8) color.Color { return color.NRGBA{R: r, G: g, B: b, A: 0xff} }

// alpha returns c at the given opacity. Fyne asks for translucent colors in
// places KDE has no role for — hover fills, the scrollbar, the modal overlay —
// and deriving them from a scheme color keeps them in the scheme's hue instead
// of dropping a neutral grey onto a warm palette.
func alpha(c color.Color, a uint8) color.Color {
	// RGBA returns 16-bit channels; the high byte is the 8-bit value, so the
	// shift cannot overflow and the conversion is exact.
	r, g, b, _ := c.RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: a} //nolint:gosec // r,g,b are 16-bit; >>8 fits a byte
}

// BreezeDark is /usr/share/color-schemes/BreezeDark.colors.
var breezeDark = palette{
	name: "Breeze Dark", dark: true,
	windowBG: rgb(32, 35, 38), windowFG: rgb(252, 252, 252),
	viewBG: rgb(20, 22, 24), viewAltBG: rgb(29, 31, 34), viewFG: rgb(252, 252, 252),
	buttonBG: rgb(41, 44, 48), buttonFG: rgb(252, 252, 252),
	selectionBG: rgb(61, 174, 233), selectionFG: rgb(252, 252, 252),
	tooltipBG:  rgb(41, 44, 48),
	inactiveFG: rgb(161, 169, 177),
	negative:   rgb(218, 68, 83), positive: rgb(39, 174, 96), neutral: rgb(246, 116, 0),
	link: rgb(29, 153, 243), focus: rgb(61, 174, 233), hover: rgb(61, 174, 233),
	separator: rgb(60, 64, 69),
}

// BreezeLight is /usr/share/color-schemes/BreezeLight.colors.
var breezeLight = palette{
	name: "Breeze Light", dark: false,
	windowBG: rgb(239, 240, 241), windowFG: rgb(35, 38, 41),
	viewBG: rgb(255, 255, 255), viewAltBG: rgb(247, 247, 247), viewFG: rgb(35, 38, 41),
	buttonBG: rgb(252, 252, 252), buttonFG: rgb(35, 38, 41),
	selectionBG: rgb(61, 174, 233), selectionFG: rgb(255, 255, 255),
	tooltipBG:  rgb(247, 247, 247),
	inactiveFG: rgb(112, 125, 138),
	negative:   rgb(218, 68, 83), positive: rgb(39, 174, 96), neutral: rgb(246, 116, 0),
	link: rgb(41, 128, 185), focus: rgb(61, 174, 233), hover: rgb(61, 174, 233),
	separator: rgb(200, 203, 207),
}

// OxygenDark is /usr/share/color-schemes/OxygenDark.colors. Oxygen is the warm
// scheme: its greys carry red, its foregrounds are off-white towards peach, and
// its selection is orange rather than blue. Transcribing it faithfully rather
// than tinting Breeze Dark is the point — a scheme that reads as "the same dark
// grey again" would not tell us whether Fyne can carry a scheme at all.
var oxygenDark = palette{
	name: "Oxygen Dark", dark: true,
	windowBG: rgb(38, 36, 35), windowFG: rgb(255, 232, 223),
	viewBG: rgb(30, 29, 29), viewAltBG: rgb(33, 31, 30), viewFG: rgb(255, 230, 222),
	buttonBG: rgb(57, 53, 50), buttonFG: rgb(255, 234, 225),
	selectionBG: rgb(247, 159, 82), selectionFG: rgb(38, 36, 35),
	tooltipBG:  rgb(24, 21, 19),
	inactiveFG: rgb(137, 136, 135),
	// Oxygen's own negative/positive are very dark (191,3,3 / 0,110,40) because
	// the scheme was authored for a light window. On the dark variant they are
	// close to unreadable against the view background, so they are lightened
	// here. This is the one place the transcription is not literal, and it is a
	// legibility fix, not a taste preference.
	negative: rgb(232, 87, 82), positive: rgb(94, 189, 110), neutral: rgb(226, 170, 61),
	link: rgb(88, 172, 255), focus: rgb(240, 213, 194), hover: rgb(255, 229, 208),
	separator: rgb(64, 60, 57),
}

// Adwaita is GNOME's scheme rather than KDE's, and unlike the three above it is
// not transcribed from a file on disk — libadwaita ships named colors compiled
// into the library, not a .colors document. These values are its documented
// named colors (window_bg_color, view_bg_color, accent_bg_color, and the
// destructive/success/warning triple). Where libadwaita specifies a translucent
// color over another — its buttons are black at 5% over the window — the
// composited result is used, because Fyne asks for an opaque fill there.
var adwaitaLight = palette{
	name: "Adwaita Light", dark: false,
	windowBG: rgb(250, 250, 250), windowFG: rgb(46, 52, 54),
	viewBG: rgb(255, 255, 255), viewAltBG: rgb(246, 245, 244), viewFG: rgb(46, 52, 54),
	buttonBG: rgb(239, 239, 239), buttonFG: rgb(46, 52, 54),
	selectionBG: rgb(53, 132, 228), selectionFG: rgb(255, 255, 255),
	tooltipBG:  rgb(53, 57, 60),
	inactiveFG: rgb(146, 149, 149),
	negative:   rgb(224, 27, 36), positive: rgb(38, 162, 105), neutral: rgb(229, 165, 10),
	link: rgb(28, 113, 216), focus: rgb(53, 132, 228), hover: rgb(53, 132, 228),
	separator: rgb(205, 199, 194),
}

var adwaitaDark = palette{
	name: "Adwaita Dark", dark: true,
	windowBG: rgb(36, 36, 36), windowFG: rgb(255, 255, 255),
	viewBG: rgb(30, 30, 30), viewAltBG: rgb(48, 48, 48), viewFG: rgb(255, 255, 255),
	buttonBG: rgb(56, 56, 56), buttonFG: rgb(255, 255, 255),
	selectionBG: rgb(53, 132, 228), selectionFG: rgb(255, 255, 255),
	tooltipBG:  rgb(56, 56, 56),
	inactiveFG: rgb(154, 153, 150),
	negative:   rgb(255, 122, 116), positive: rgb(46, 194, 126), neutral: rgb(245, 194, 17),
	link: rgb(120, 174, 237), focus: rgb(53, 132, 228), hover: rgb(53, 132, 228),
	separator: rgb(61, 61, 61),
}

var palettes = []palette{breezeDark, breezeLight, oxygenDark, adwaitaDark, adwaitaLight}

// paletteNames lists the schemes in the order they are offered.
func paletteNames() []string {
	names := make([]string, 0, len(palettes))
	for _, p := range palettes {
		names = append(names, p.name)
	}
	return names
}

// paletteByName returns the named scheme, falling back to the first one. A
// missing name means a stale preference, not an error worth a dialog.
func paletteByName(name string) palette {
	for _, p := range palettes {
		if p.name == name {
			return p
		}
	}
	return palettes[0]
}

// kdeTheme adapts a desktop color scheme to fyne.Theme, together with the
// user's chosen font and text size. Icons come from the default theme: a color
// scheme says nothing about them, and following the desktop's icon theme would
// mean reading it at runtime.
type kdeTheme struct {
	p    palette
	font *loadedFont // nil means the font Fyne ships with
	mono *loadedFont // the console font for Monospace text; nil means Fyne's monospace face
	text float32     // text size in points; 0 means the default
}

var _ fyne.Theme = kdeTheme{}

func (t kdeTheme) Font(s fyne.TextStyle) fyne.Resource {
	// Monospace is never taken from the interface family: a proportional
	// family will not have a monospace face, and substituting one silently
	// would misalign the places that asked for monospace precisely because
	// alignment mattered. It comes from the console font (clockwork-orange.yml
	// console_font_family, set in Appearance) when one is chosen, else Fyne's.
	if s.Monospace {
		if r := t.mono.face(s); r != nil {
			return r
		}
		return theme.DefaultTheme().Font(s)
	}
	if r := t.font.face(s); r != nil {
		return r
	}
	return theme.DefaultTheme().Font(s)
}

func (t kdeTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(n)
}

func (t kdeTheme) Size(n fyne.ThemeSizeName) float32 {
	// Breeze is a tighter, squarer style than Fyne's default. Pulling the
	// corner radius and padding in is what stops the window reading as
	// "Fyne wearing KDE colors".
	switch n {
	case theme.SizeNameInputRadius, theme.SizeNameSelectionRadius:
		return 2
	case theme.SizeNamePadding:
		return 3
	case theme.SizeNameText:
		if t.text > 0 {
			return t.text
		}
		return defaultTextSize
	case theme.SizeNameHeadingText:
		return t.textSize() * 1.3
	case theme.SizeNameSubHeadingText:
		return t.textSize() * 1.15
	case theme.SizeNameCaptionText:
		return t.textSize() * 0.85
	}
	return theme.DefaultTheme().Size(n)
}

func (t kdeTheme) Color(n fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	// The variant argument is ignored on purpose. Fyne passes the variant the
	// app is set to, but the scheme already decided whether it is dark; honouring
	// both would let a mismatch paint dark text on a dark window.
	p := t.p
	switch n {
	case theme.ColorNameBackground:
		return p.windowBG
	case theme.ColorNameForeground:
		return p.windowFG
	case theme.ColorNameForegroundOnPrimary:
		return p.selectionFG
	case theme.ColorNameInputBackground, theme.ColorNameMenuBackground:
		return p.viewBG
	case theme.ColorNameOverlayBackground:
		return p.windowBG
	case theme.ColorNameButton:
		return p.buttonBG
	case theme.ColorNamePrimary, theme.ColorNameSelection:
		return p.selectionBG
	case theme.ColorNameFocus:
		return alpha(p.focus, 0x66)
	case theme.ColorNameHover:
		return alpha(p.hover, 0x33)
	case theme.ColorNamePressed:
		return alpha(p.selectionBG, 0x55)
	case theme.ColorNameDisabled, theme.ColorNamePlaceHolder:
		return p.inactiveFG
	case theme.ColorNameDisabledButton:
		return alpha(p.buttonBG, 0x88)
	case theme.ColorNameError:
		return p.negative
	case theme.ColorNameSuccess:
		return p.positive
	case theme.ColorNameWarning:
		return p.neutral
	case theme.ColorNameHyperlink:
		return p.link
	case theme.ColorNameSeparator, theme.ColorNameInputBorder:
		return p.separator
	case theme.ColorNameHeaderBackground:
		return p.viewAltBG
	case theme.ColorNameScrollBar:
		return alpha(p.inactiveFG, 0x99)
	case theme.ColorNameShadow:
		return color.NRGBA{A: 0x66}
	}
	return theme.DefaultTheme().Color(n, t.variant())
}

// defaultTextSize is smaller than Fyne's own 14. This is a dense, information-
// heavy window — tables, reports, scan listings — and 14pt makes it feel like a
// phone application. 12 is close to what Breeze and Adwaita use for interface
// text at a normal DPI.
const defaultTextSize float32 = 12

// textSizes are the choices offered in Appearance.
var textSizes = []float32{10, 11, 12, 13, 14, 16, 18}

func (t kdeTheme) textSize() float32 {
	if t.text > 0 {
		return t.text
	}
	return defaultTextSize
}

func (t kdeTheme) variant() fyne.ThemeVariant {
	if t.p.dark {
		return theme.VariantDark
	}
	return theme.VariantLight
}
