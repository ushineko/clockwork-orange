package imaging

import (
	"fmt"
	"image"
	"image/png"
	"os"
)

/*
Trimming the flat bars some sources bake into a wallpaper.

An image arrives at the right resolution with white down the left and right, or
black across the top and bottom: a picture of another aspect ratio, padded to
fit rather than cropped to it. The cover-crop preserves the padding, so it
reaches the desktop.

Detecting the bar is easy and detecting it *safely* is the whole problem. A
naive scan -- walk inward while the line is flat and extreme -- marches straight
through the flat regions of a dark space scene or a minimalist design. Over the
author's 638 downloaded wallpapers it claimed bars on 70 of them, including
L2736 R2736 on a 5760-pixel-wide picture, which is not a bar but the image.

Three guardrails bring that to 9, all of them real (spec 019):

  - a bar may not exceed maxBarFraction of its dimension;
  - it must end at a hard step in luma, because padding meets content at an
    edge and a flat region of content fades into its neighbours;
  - what is left must still be large enough to be a wallpaper.

The bars this is aimed at are exact: the measured example is luma 255.0 with a
maximum deviation of 0.0 across the whole column, ending at a column whose
deviation is 134. The thresholds below are loose enough for JPEG ringing and
nowhere near loose enough to reach content.
*/

const (
	// maxBarFraction caps a bar at a quarter of its dimension. Padding to a
	// common aspect ratio never needs more: 16:9 from 4:3 is 12.5% a side,
	// from 1:1 it is 21.9%.
	maxBarFraction = 4 // i.e. 1/4

	// barFlatness is how much a line may vary and still be flat. JPEG ringing
	// against a hard edge moves a pure bar by a few counts.
	barFlatness = 12.0

	// barBright and barDark are how extreme a flat line must be to be padding
	// rather than sky or shadow.
	barBright = 225.0
	barDark   = 24.0

	// barStep is the luma jump required where the bar meets the picture.
	barStep = 40.0

	// minKeptFraction is how much of each dimension must survive, exclusive.
	// Both sides hitting the cap leaves exactly half, which is the worst case
	// the other guardrails allow through -- and an image that is half padding
	// is one this should not be deciding about on its own.
	minKeptFraction = 0.5

	// minBar is the smallest run worth calling padding. A one- to four-pixel
	// flat edge is a scaling artefact, not a letterbox: removing it changes
	// nothing a viewer can see, and for a plugin that would have to re-encode
	// the file to do it, that is a rewrite for no reason.
	minBar = 8

	// barSample is the stride along a line. A bar is uniform by definition, so
	// every fourth pixel says as much as every pixel and says it faster.
	barSample = 4
)

// lumaAt is the BT.601 luma of one pixel, the same weighting the review's
// overlay and the thumbnailer use.
func lumaAt(img image.Image, x, y int) float64 {
	r, g, b, _ := img.At(x, y).RGBA()
	return 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8)
}

// lineLuma is the mean luma of a row or column, and the largest deviation from
// that mean: flatness and level in one pass.
func lineLuma(img image.Image, fixed int, vertical bool) (mean, dev float64) {
	b := img.Bounds()
	var vals []float64
	if vertical {
		for y := b.Min.Y; y < b.Max.Y; y += barSample {
			vals = append(vals, lumaAt(img, fixed, y))
		}
	} else {
		for x := b.Min.X; x < b.Max.X; x += barSample {
			vals = append(vals, lumaAt(img, x, fixed))
		}
	}
	if len(vals) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	mean = sum / float64(len(vals))
	for _, v := range vals {
		d := v - mean
		if d < 0 {
			d = -d
		}
		if d > dev {
			dev = d
		}
	}
	return mean, dev
}

// isBarLine reports whether a row or column is flat enough and extreme enough
// to be padding.
func isBarLine(img image.Image, fixed int, vertical bool) bool {
	mean, dev := lineLuma(img, fixed, vertical)
	return dev < barFlatness && (mean > barBright || mean < barDark)
}

// stepAt reports whether the luma jumps at the boundary between the bar and
// what follows it: the edge that separates padding from a flat picture.
func stepAt(img image.Image, inner int, vertical bool) bool {
	b := img.Bounds()
	if vertical && (inner-1 < b.Min.X || inner >= b.Max.X) {
		return false
	}
	if !vertical && (inner-1 < b.Min.Y || inner >= b.Max.Y) {
		return false
	}
	outer, _ := lineLuma(img, inner-1, vertical)
	in, _ := lineLuma(img, inner, vertical)
	d := outer - in
	if d < 0 {
		d = -d
	}
	return d > barStep
}

// run counts the bar lines from one edge, stopping at the cap.
func run(img image.Image, from, to, step, limit int, vertical bool) int {
	n := 0
	for i := from; n < limit && i != to; i += step {
		if !isBarLine(img, i, vertical) {
			break
		}
		n++
	}
	return n
}

/*
TrimBars returns the content rectangle of src with flat padding removed, and
whether anything was trimmed.

Conservative by construction: every guardrail that fails returns the original
bounds, so a picture this cannot confidently read is a picture it leaves alone.
The caller crops to the returned rectangle and carries on; nothing is written
and the source image is not modified.
*/
func TrimBars(src image.Image) (image.Rectangle, bool) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 8 || h < 8 {
		return b, false
	}
	limitX, limitY := w/maxBarFraction, h/maxBarFraction

	left := run(src, b.Min.X, b.Max.X, 1, limitX, true)
	right := run(src, b.Max.X-1, b.Min.X-1, -1, limitX, true)
	top := run(src, b.Min.Y, b.Max.Y, 1, limitY, false)
	bottom := run(src, b.Max.Y-1, b.Min.Y-1, -1, limitY, false)

	// A run too short to see is an artefact, not padding.
	if left < minBar {
		left = 0
	}
	if right < minBar {
		right = 0
	}
	if top < minBar {
		top = 0
	}
	if bottom < minBar {
		bottom = 0
	}

	// A side whose bar does not end at a step was never a bar.
	if left > 0 && !stepAt(src, b.Min.X+left, true) {
		left = 0
	}
	if right > 0 && !stepAt(src, b.Max.X-right, true) {
		right = 0
	}
	if top > 0 && !stepAt(src, b.Min.Y+top, false) {
		top = 0
	}
	if bottom > 0 && !stepAt(src, b.Max.Y-bottom, false) {
		bottom = 0
	}
	if left+right+top+bottom == 0 {
		return b, false
	}

	kept := image.Rect(b.Min.X+left, b.Min.Y+top, b.Max.X-right, b.Max.Y-bottom)
	if float64(kept.Dx()) <= float64(w)*minKeptFraction || float64(kept.Dy()) <= float64(h)*minKeptFraction {
		return b, false // this is not padding around a picture
	}
	return kept, true
}

/*
TrimBarsFile rewrites path without its flat padding, and reports whether it
did.

For a plugin that saves the bytes it downloaded rather than re-encoding them
(Wallhaven keeps the original file, PNG and all). Decoding to look costs a
tenth of a second; re-encoding happens only for the one image in seventy that
turns out to be padded, so the other sixty-nine keep the bytes the source sent.

The file is re-encoded in the format it already was, so a PNG stays a PNG and
nothing acquires JPEG artefacts it did not have. A format this package cannot
write back is left alone: trimming is worth less than the original.
*/
func TrimBarsFile(path string, jpegQuality int) (bool, error) {
	src, format, err := DecodeFile(path)
	if err != nil {
		return false, err
	}
	kept, trimmed := TrimBars(src)
	if !trimmed {
		return false, nil
	}
	sub, ok := src.(interface {
		SubImage(r image.Rectangle) image.Image
	})
	var out image.Image
	if ok {
		out = sub.SubImage(kept)
	} else {
		dst := image.NewRGBA(image.Rect(0, 0, kept.Dx(), kept.Dy()))
		for y := 0; y < kept.Dy(); y++ {
			for x := 0; x < kept.Dx(); x++ {
				dst.Set(x, y, src.At(kept.Min.X+x, kept.Min.Y+y))
			}
		}
		out = dst
	}
	switch format {
	case "jpeg":
		return true, SaveJPEG(path, out, jpegQuality)
	case "png":
		return true, savePNG(path, out)
	}
	return false, nil // a format this package does not write back
}

// savePNG writes img as PNG, the companion to SaveJPEG for a source that was
// one. Kept here rather than in imaging.go because trimming is the only thing
// in this package that writes a PNG back.
func savePNG(path string, img image.Image) error {
	f, err := os.Create(path) //nolint:gosec // the path the caller already wrote
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}
