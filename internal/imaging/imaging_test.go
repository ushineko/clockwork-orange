package imaging

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// goldenHash is one entry of tests/golden/images/hashes.json.
type goldenHash struct {
	MD5             string `json:"md5"`
	SHA256          string `json:"sha256"`
	ThumbnailFormat string `json:"thumbnail_format"`
	ThumbnailSize   []int  `json:"thumbnail_size"`
}

func loadGoldenHashes(t *testing.T) map[string]goldenHash {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(goldenImages(t), "hashes.json"))
	require.NoError(t, err)
	var m map[string]goldenHash
	require.NoError(t, json.Unmarshal(raw, &m))
	require.NotEmpty(t, m)
	return m
}

func TestMD5FileMatchesPythonHistoryHashForEveryFixture(t *testing.T) {
	for name, want := range loadGoldenHashes(t) {
		got, err := MD5File(filepath.Join(goldenImages(t), name))
		require.NoError(t, err, name)
		require.Equal(t, want.MD5, got, name)
	}
}

func TestSHA256FileMatchesPythonBlacklistHashForEveryFixture(t *testing.T) {
	for name, want := range loadGoldenHashes(t) {
		got, err := SHA256File(filepath.Join(goldenImages(t), name))
		require.NoError(t, err, name)
		require.Equal(t, want.SHA256, got, name)
	}
}

func TestSHA256FileStreamsFilesLargerThanOneBlockCorrectly(t *testing.T) {
	// 10000 bytes is not a multiple of 4096, so the final block is short;
	// expected digests computed independently with Python's hashlib.
	p := filepath.Join(t.TempDir(), "blob.bin")
	require.NoError(t, os.WriteFile(p, bytes.Repeat([]byte{0xab}, 10000), 0o600))
	got, err := SHA256File(p)
	require.NoError(t, err)
	require.Equal(t, "753371ecea131ec9f1801c3891b27718e70bcbddcf77134434cb0888bda9857a", got)
	got, err = MD5File(p)
	require.NoError(t, err)
	require.Equal(t, "88dc3354762f2379a5b39c2e1a49fb26", got)
}

func TestHashFunctionsReportMissingFile(t *testing.T) {
	_, err := MD5File(filepath.Join(t.TempDir(), "nope"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = SHA256File(filepath.Join(t.TempDir(), "nope"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestThumbnailDimensionsEqualPillowThumbnail128ForEveryFixture(t *testing.T) {
	for name, want := range loadGoldenHashes(t) {
		data, err := Thumbnail(filepath.Join(goldenImages(t), name), 128, 70)
		require.NoError(t, err, name)
		img, format, err := image.Decode(bytes.NewReader(data))
		require.NoError(t, err, name)
		require.Equal(t, "jpeg", format, name)
		require.Equal(t, want.ThumbnailSize, []int{img.Bounds().Dx(), img.Bounds().Dy()}, name)
	}
}

func TestThumbnailSizeNeverUpscalesAndRoundsLikePillow(t *testing.T) {
	cases := []struct {
		name         string
		sw, sh       int
		wantW, wantH int
	}{
		{"already smaller stays", 100, 50, 100, 50},
		{"exactly max stays", 128, 128, 128, 128},
		{"landscape", 1920, 1080, 128, 72},
		{"portrait", 1080, 1920, 72, 128},
		{"square", 600, 600, 128, 128},
		// plain floor would give 127; Pillow's round_aspect picks the ceil
		// because 128/128 is closer to 1000/999 than 128/127 is.
		{"near-square rounds toward source aspect", 1000, 999, 128, 128},
		{"extreme aspect clamps to 1", 10000, 10, 128, 1},
		{"one side over max", 200, 64, 128, 41},
	}
	for _, c := range cases {
		w, h := thumbnailSize(c.sw, c.sh, 128, 128)
		require.Equal(t, [2]int{c.wantW, c.wantH}, [2]int{w, h}, c.name)
	}
}

func TestThumbnailRejectsNonPositiveSizeAndNonImages(t *testing.T) {
	_, err := Thumbnail(filepath.Join(goldenImages(t), "square_600x600.png"), 0, 70)
	require.Error(t, err)
	p := filepath.Join(t.TempDir(), "notimage.png")
	require.NoError(t, os.WriteFile(p, []byte("hello"), 0o600))
	_, err = Thumbnail(p, 128, 70)
	require.Error(t, err)
}

func TestToRGBDropsAlphaWithoutDarkeningLikePillowConvertRGB(t *testing.T) {
	// The rgba fixture has alpha 128 everywhere; Pillow's convert("RGB")
	// keeps the colour bytes verbatim. A premultiplied draw would halve them.
	src, _, err := DecodeFile(filepath.Join(goldenImages(t), "rgba_400x300.png"))
	require.NoError(t, err)
	nrgba, ok := src.(*image.NRGBA)
	require.True(t, ok, "fixture should decode as NRGBA")
	got := ToRGB(src)
	for _, pt := range []image.Point{{0, 0}, {13, 200}, {399, 299}} {
		want := nrgba.NRGBAAt(pt.X, pt.Y)
		require.Equal(t, uint8(128), want.A, "fixture alpha")
		require.Equal(t, color.RGBA{R: want.R, G: want.G, B: want.B, A: 0xff}, got.RGBAAt(pt.X, pt.Y), pt)
	}

	// Generic path: a premultiplied RGBA source is un-premultiplied.
	prem := image.NewRGBA(image.Rect(0, 0, 1, 1))
	prem.SetRGBA(0, 0, color.RGBA{R: 50, G: 100, B: 0, A: 100})
	c := ToRGB(prem).RGBAAt(0, 0)
	require.Equal(t, uint8(0xff), c.A)
	require.InDelta(t, 127, int(c.R), 1)
	require.InDelta(t, 255, int(c.G), 1)
}

func TestCoverResizeCropYieldsExactTargetForEveryAspect(t *testing.T) {
	for _, name := range []string{"landscape_1920x1080.png", "portrait_1080x1920.png", "square_600x600.png", "rgba_400x300.png"} {
		src, _, err := DecodeFile(filepath.Join(goldenImages(t), name))
		require.NoError(t, err, name)
		for _, target := range [][2]int{{1920, 1080}, {1080, 1920}, {600, 600}} {
			out := CoverResizeCrop(src, target[0], target[1])
			require.Equal(t, target, [2]int{out.Bounds().Dx(), out.Bounds().Dy()}, "%s -> %v", name, target)
			require.True(t, out.Opaque(), "%s -> %v must be opaque", name, target)
		}
	}
}

func TestIsImageFileUsesExtensionSetThenMimePrefix(t *testing.T) {
	require.True(t, IsImageFile("/a/b/photo.jpg"))
	require.True(t, IsImageFile("/a/b/PHOTO.JPG"), "extension match is case-insensitive")
	require.True(t, IsImageFile("wall.WebP"))
	require.True(t, IsImageFile("vector.svg"))
	require.True(t, IsImageFile("new.avif"), "not in the set, but Go's builtin mime table says image/avif")
	require.False(t, IsImageFile("notes.txt"))
	require.False(t, IsImageFile("archive.tar.gz"))
	require.False(t, IsImageFile("README"))
	require.False(t, IsImageFile(""))
}

func TestEncodeJPEGRoundTripsThroughDecode(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	var buf bytes.Buffer
	require.NoError(t, EncodeJPEG(&buf, img, 70))
	_, format, err := Decode(&buf)
	require.NoError(t, err)
	require.Equal(t, "jpeg", format)
	var pngBuf bytes.Buffer
	require.NoError(t, png.Encode(&pngBuf, img))
	_, format, err = Decode(&pngBuf)
	require.NoError(t, err)
	require.Equal(t, "png", format)
}
