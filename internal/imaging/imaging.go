/*
Package imaging is the small set of pixel operations the port needs (spec 010
R3.7): decoding the formats the wallpaper engine accepts, Pillow-equivalent
cover-resize-and-crop, thumbnails, JPEG encoding, and the two file hashes the
stores use.

Resampling uses x/image/draw.CatmullRom as the stand-in for Pillow's LANCZOS.
The outputs are not byte-identical to Pillow's (different kernels, different
JPEG encoders); the golden tests therefore pin dimensions, formats and hashes
of *inputs*, never encoded bytes.
*/
package imaging

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"os"

	xdraw "golang.org/x/image/draw"

	_ "image/gif" // decoder registration (R3.1)
	_ "image/png" // decoder registration (R3.1)

	_ "golang.org/x/image/bmp"  // decoder registration (R3.1)
	_ "golang.org/x/image/tiff" // decoder registration (R3.1)
	_ "golang.org/x/image/webp" // decoder registration (R3.1)
)

// Decode reads any registered image format.
func Decode(r io.Reader) (image.Image, string, error) {
	img, format, err := image.Decode(r)
	if err != nil {
		return nil, "", fmt.Errorf("decode image: %w", err)
	}
	return img, format, nil
}

// DecodeFile opens and decodes a file.
func DecodeFile(path string) (image.Image, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("open image: %w", err)
	}
	defer func() { _ = f.Close() }()
	return Decode(f)
}

/*
ToRGB flattens any image to an opaque RGB raster the way Pillow's
convert("RGB") does (R3.7): the alpha channel is dropped, not composited.

Colour channels are read in non-premultiplied form (Pillow's RGBA is
non-premultiplied), so a half-transparent pixel keeps its colour instead of
being darkened. The NRGBA fast path covers PNGs with alpha, the format the
concern actually arises for; everything else goes through color.NRGBAModel,
which is the identity for opaque pixels.
*/
func ToRGB(src image.Image) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if s, ok := src.(*image.NRGBA); ok {
		for y := 0; y < h; y++ {
			si := s.PixOffset(b.Min.X, b.Min.Y+y)
			copy(dst.Pix[y*dst.Stride:y*dst.Stride+w*4], s.Pix[si:si+w*4])
		}
	} else {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				c := color.NRGBAModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
				i := y*dst.Stride + x*4
				dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2] = c.R, c.G, c.B
			}
		}
	}
	setOpaque(dst)
	return dst
}

// setOpaque forces every alpha byte of img to 0xff.
func setOpaque(img *image.RGBA) {
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 0xff
	}
}

// flatten returns src unchanged when it is known to be fully opaque and an
// RGB-flattened copy otherwise, mirroring the Python code's convert("RGB")
// before every resize (platform_utils.py, duckduckgo_images.py).
func flatten(src image.Image) image.Image {
	if o, ok := src.(interface{ Opaque() bool }); ok && o.Opaque() {
		return src
	}
	return ToRGB(src)
}

/*
CoverResizeCrop scales src so that it covers w×h and crops the centre to
exactly w×h -- Pillow's "cover fit" as written in platform_utils.py and
duckduckgo_images.py:

	if aspect_img > aspect_target: new_h = h; new_w = aspect_img * new_h
	else:                          new_w = w; new_h = new_w / aspect_img

followed by a centred crop. The intermediate is rounded to int the way Python's
int() truncates, then clamped so the crop window always fits. Like the Python
code, the source is flattened to RGB before scaling.
*/
func CoverResizeCrop(src image.Image, w, h int) *image.RGBA {
	if w <= 0 || h <= 0 {
		return image.NewRGBA(image.Rect(0, 0, 0, 0))
	}
	src = flatten(src)
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw == 0 || sh == 0 {
		return image.NewRGBA(image.Rect(0, 0, w, h))
	}
	aspectImg := float64(sw) / float64(sh)
	aspectTarget := float64(w) / float64(h)
	var nw, nh int
	if aspectImg > aspectTarget {
		nh = h
		nw = int(aspectImg * float64(nh))
	} else {
		nw = w
		nh = int(float64(nw) / aspectImg)
	}
	if nw < w {
		nw = w
	}
	if nh < h {
		nh = h
	}
	scaled := image.NewRGBA(image.Rect(0, 0, nw, nh))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), src, b, xdraw.Src, nil)
	left := (nw - w) / 2
	top := (nh - h) / 2
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(out, out.Bounds(), scaled, image.Pt(left, top), draw.Src)
	setOpaque(out)
	return out
}

// EncodeJPEG writes img as JPEG at the given quality (1-100).
func EncodeJPEG(w io.Writer, img image.Image, quality int) error {
	if err := jpeg.Encode(w, img, &jpeg.Options{Quality: quality}); err != nil {
		return fmt.Errorf("encode jpeg: %w", err)
	}
	return nil
}

// SaveJPEG encodes img to path at the given quality.
func SaveJPEG(path string, img image.Image, quality int) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if err := EncodeJPEG(f, img, quality); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}
