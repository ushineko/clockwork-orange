package imaging

import (
	"bytes"
	"fmt"
	"image"
	"math"

	xdraw "golang.org/x/image/draw"
)

/*
Thumbnail decodes path and returns it as JPEG bytes fitting within
maxSide×maxSide (R3.7, R5.6) with Pillow's Image.thumbnail((n, n))
semantics: aspect preserved, never upscaled, an image already small enough
keeps its size. The image is flattened to RGB first (plugins/blacklist.py
converted RGBA/P to RGB before saving).
*/
func Thumbnail(path string, maxSide, quality int) ([]byte, error) {
	if maxSide <= 0 {
		return nil, fmt.Errorf("thumbnail: maxSide must be positive, got %d", maxSide)
	}
	src, _, err := DecodeFile(path)
	if err != nil {
		return nil, err
	}
	rgb := ToRGB(src)
	sw, sh := rgb.Bounds().Dx(), rgb.Bounds().Dy()
	if sw == 0 || sh == 0 {
		return nil, fmt.Errorf("thumbnail: %s has no pixels", path)
	}
	w, h := thumbnailSize(sw, sh, maxSide, maxSide)
	img := rgb
	if w != sw || h != sh {
		img = image.NewRGBA(image.Rect(0, 0, w, h))
		xdraw.CatmullRom.Scale(img, img.Bounds(), rgb, rgb.Bounds(), xdraw.Src, nil)
		setOpaque(img)
	}
	var buf bytes.Buffer
	if err := EncodeJPEG(&buf, img, quality); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

/*
thumbnailSize is a port of the size arithmetic in Pillow's Image.thumbnail
(preserve_aspect_ratio / round_aspect, Pillow 12.1): the output fits within
maxW×maxH, is never larger than the source, and the shorter side is rounded
to whichever of floor/ceil gives the aspect ratio closest to the source's
(floor on ties), with a minimum of 1.

Ported verbatim rather than simplified: a plain floor gives 128×127 for a
1000×999 source where Pillow gives 128×128, and the golden thumbnail sizes
in tests/golden/images/hashes.json are Pillow's.
*/
func thumbnailSize(srcW, srcH, maxW, maxH int) (int, int) {
	if maxW >= srcW && maxH >= srcH {
		return srcW, srcH
	}
	aspect := float64(srcW) / float64(srcH)
	x, y := maxW, maxH
	fx, fy := float64(x), float64(y)
	if fx/fy >= aspect {
		x = roundAspect(fy*aspect, func(n int) float64 { return math.Abs(aspect - float64(n)/fy) })
	} else {
		y = roundAspect(fx/aspect, func(n int) float64 {
			if n == 0 {
				return 0
			}
			return math.Abs(aspect - fx/float64(n))
		})
	}
	return x, y
}

// roundAspect is Pillow's round_aspect: the better of floor/ceil under key,
// floor winning ties (Python's min keeps the first minimum), at least 1.
func roundAspect(number float64, key func(int) float64) int {
	lo, hi := int(math.Floor(number)), int(math.Ceil(number))
	best := lo
	if key(hi) < key(lo) {
		best = hi
	}
	if best < 1 {
		return 1
	}
	return best
}
