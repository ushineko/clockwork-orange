package imaging

import (
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/require"
)

// padded draws content of the given size inside bars of the given colour.
func padded(w, h, left, right, top, bottom int, bar color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, bar)
		}
	}
	// Content: a gradient, so no line of it is flat.
	for y := top; y < h-bottom; y++ {
		for x := left; x < w-right; x++ {
			v := uint8((x*7 + y*13) % 200)
			img.Set(x, y, color.RGBA{R: v, G: uint8(255 - int(v)), B: 128, A: 255})
		}
	}
	return img
}

var white = color.RGBA{R: 255, G: 255, B: 255, A: 255}
var black = color.RGBA{A: 255}

// The case that prompted this: pillarboxed, white, ~5% a side (spec 019).
func TestTrimBarsRemovesPillarboxing(t *testing.T) {
	img := padded(3840, 2160, 191, 187, 0, 0, white)
	kept, trimmed := TrimBars(img)
	require.True(t, trimmed)
	require.Equal(t, image.Rect(191, 0, 3653, 2160), kept)
	require.Equal(t, 3462, kept.Dx())
}

// Letterboxing, and black bars, are the same thing the other way up.
func TestTrimBarsRemovesLetterboxing(t *testing.T) {
	img := padded(3840, 2160, 0, 0, 264, 264, black)
	kept, trimmed := TrimBars(img)
	require.True(t, trimmed)
	require.Equal(t, image.Rect(0, 264, 3840, 1896), kept)
}

/*
A picture with no padding is returned untouched.

The guardrail that matters most. A naive version of this claimed bars on 70 of
the author's 638 wallpapers, including 2736 pixels a side on a 5760-wide one,
because it marched inward through the flat regions of dark and minimalist
images. Those are the cases below.
*/
func TestTrimBarsLeavesAPictureAlone(t *testing.T) {
	cases := map[string]image.Image{
		"a gradient with no bars at all": padded(1920, 1080, 0, 0, 0, 0, white),
		// Mostly black, fading into content: flat at the edge, but no step.
		"a dark scene that fades into its subject": func() image.Image {
			img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
			for y := range 1080 {
				for x := range 1920 {
					v := uint8(min(255, x/8)) // a slow ramp: flat-ish, never a step
					img.Set(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
				}
			}
			return img
		}(),
		// Bars wider than the cap are not bars; this is a picture on white.
		"a minimalist picture on a white field": padded(1920, 1080, 700, 700, 0, 0, white),
	}
	for name, img := range cases {
		kept, trimmed := TrimBars(img)
		require.Falsef(t, trimmed, "%s: should not be trimmed", name)
		require.Equalf(t, img.Bounds(), kept, "%s: bounds unchanged", name)
	}
}

// Padding on all four sides comes off together.
func TestTrimBarsHandlesAllFourSides(t *testing.T) {
	img := padded(1600, 900, 80, 80, 45, 45, white)
	kept, trimmed := TrimBars(img)
	require.True(t, trimmed)
	require.Equal(t, image.Rect(80, 45, 1520, 855), kept)
}

// A picture too small to read is left alone rather than guessed at.
func TestTrimBarsIgnoresTinyImages(t *testing.T) {
	img := padded(6, 6, 1, 1, 1, 1, white)
	_, trimmed := TrimBars(img)
	require.False(t, trimmed)
}

// The trim must leave a wallpaper behind, not a stamp.
func TestTrimBarsRefusesToCropMostOfThePicture(t *testing.T) {
	// Bars at the cap on every side would keep half of each dimension; the
	// floor is half, so this is the boundary and must be refused.
	img := padded(1000, 1000, 250, 250, 250, 250, white)
	_, trimmed := TrimBars(img)
	require.False(t, trimmed, "a quarter off every side leaves half, which is the floor")
}

// A few pixels of flat edge is a scaling artefact, not padding: removing it
// changes nothing a viewer sees, and costs a file rewrite for a plugin that
// keeps its originals (spec 019).
func TestTrimBarsIgnoresAFewPixelsOfEdge(t *testing.T) {
	img := padded(3840, 2160, 4, 4, 4, 4, white)
	_, trimmed := TrimBars(img)
	require.False(t, trimmed)
}
