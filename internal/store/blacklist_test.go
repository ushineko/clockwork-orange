package store

import (
	"bytes"
	"context"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/events"
	"github.com/ushineko/clockwork-orange/internal/imaging"
)

func openTempBlacklist(t *testing.T) *Blacklist {
	t.Helper()
	b, err := OpenBlacklist(filepath.Join(t.TempDir(), "blacklist.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = b.Close() })
	return b
}

func TestAddWithFileComputesSHA256AndStoresFittingThumbnail(t *testing.T) {
	b := openTempBlacklist(t)
	img := goldenImage(t, "landscape_1920x1080.png")
	require.NoError(t, b.Add("", "local", img))

	want, err := imaging.SHA256File(img)
	require.NoError(t, err)
	require.True(t, b.IsBlacklisted(want))

	items, err := b.Items()
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, want, items[0].Hash)
	require.Equal(t, "local", items[0].Source)
	require.NotNil(t, items[0].Thumbnail)
	thumb, format, err := image.Decode(bytes.NewReader(items[0].Thumbnail))
	require.NoError(t, err)
	require.Equal(t, "jpeg", format)
	require.Equal(t, [2]int{128, 72}, [2]int{thumb.Bounds().Dx(), thumb.Bounds().Dy()})
}

func TestAddWithHashOnlyStoresNullThumbnail(t *testing.T) {
	b := openTempBlacklist(t)
	hash := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	require.NoError(t, b.Add(hash, "manual", ""))
	items, err := b.Items()
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Nil(t, items[0].Thumbnail)
	require.Equal(t, "manual", items[0].Source)
}

func TestAddWithExplicitHashAndFileKeepsGivenHashButStoresThumbnail(t *testing.T) {
	b := openTempBlacklist(t)
	require.NoError(t, b.Add("givenhash", "wallhaven", goldenImage(t, "square_600x600.png")))
	items, err := b.Items()
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "givenhash", items[0].Hash)
	require.NotNil(t, items[0].Thumbnail)
}

func TestAddWithNothingReturnsErrNoHash(t *testing.T) {
	b := openTempBlacklist(t)
	require.ErrorIs(t, b.Add("", "manual", ""), ErrNoHash)
	require.ErrorIs(t, b.Add("", "manual", filepath.Join(t.TempDir(), "missing.png")), ErrNoHash)
}

func TestAddReplacesExistingRowInsteadOfFailing(t *testing.T) {
	fixedClock(t, 1000)
	b := openTempBlacklist(t)
	require.NoError(t, b.Add("h1", "first", ""))
	fixedClock(t, 2000)
	require.NoError(t, b.Add("h1", "second", ""))
	items, err := b.Items()
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "second", items[0].Source)
	require.InDelta(t, 2000, items[0].Timestamp, 1e-9)
}

func TestUnreadableImageStillBlacklistsHashWithNullThumbnail(t *testing.T) {
	// Python's generate_thumbnail returned None on failure and the row was
	// written anyway; the hash is what protects the user.
	b := openTempBlacklist(t)
	p := filepath.Join(t.TempDir(), "corrupt.png")
	require.NoError(t, os.WriteFile(p, []byte("not a png"), 0o600))
	require.NoError(t, b.Add("", "local", p))
	items, err := b.Items()
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Nil(t, items[0].Thumbnail)
	require.True(t, b.IsBlacklisted(items[0].Hash))
}

func TestRemoveDeletesRowAndTolerateUnknownHash(t *testing.T) {
	b := openTempBlacklist(t)
	require.NoError(t, b.Add("h1", "x", ""))
	require.NoError(t, b.Remove("h1"))
	require.False(t, b.IsBlacklisted("h1"))
	require.NoError(t, b.Remove("never-there"))
}

func TestIsBlacklistedReturnsFalseOnClosedDatabase(t *testing.T) {
	b, err := OpenBlacklist(filepath.Join(t.TempDir(), "blacklist.db"))
	require.NoError(t, err)
	require.NoError(t, b.Add("h1", "x", ""))
	require.True(t, b.IsBlacklisted("h1"))
	require.NoError(t, b.Close())
	require.False(t, b.IsBlacklisted("h1"), "errors must read as not blacklisted")
}

func TestItemsOrderNewestFirstAndFormatUnknownForMissingTimestamp(t *testing.T) {
	b := openTempBlacklist(t)
	fixedClock(t, 1000)
	require.NoError(t, b.Add("older", "x", ""))
	fixedClock(t, 3000)
	require.NoError(t, b.Add("newest", "x", ""))
	fixedClock(t, 2000)
	require.NoError(t, b.Add("middle", "x", ""))
	_, err := b.db.ExecContext(context.Background(), `INSERT INTO blacklist (img_hash, source, timestamp, thumbnail) VALUES ('nots', NULL, NULL, NULL)`)
	require.NoError(t, err)

	items, err := b.Items()
	require.NoError(t, err)
	var hashes []string
	for _, it := range items {
		hashes = append(hashes, it.Hash)
	}
	require.Equal(t, []string{"newest", "middle", "older", "nots"}, hashes)
	require.Equal(t, "Unknown", items[3].Date)
	require.Zero(t, items[3].Timestamp)
	require.Empty(t, items[3].Source)
	require.Equal(t, dateFor(3000, time.Local), items[0].Date)
}

func TestDateForMatchesPythonStrftimeInGivenLocation(t *testing.T) {
	require.Equal(t, "2023-11-14 22:13", dateFor(1700000000.5, time.UTC))
	require.Equal(t, "Unknown", dateFor(0, time.UTC))
	ny, err := time.LoadLocation("America/New_York")
	if err == nil {
		require.Equal(t, "2023-11-14 17:13", dateFor(1700000000.5, ny))
	}
}

func TestProcessFilesBlacklistsThenRemovesEachExistingFile(t *testing.T) {
	b := openTempBlacklist(t)
	a := copyFile(t, goldenImage(t, "small_800x600.jpg"), "a.jpg")
	c := copyFile(t, goldenImage(t, "square_600x600.png"), "c.png")
	wantA, err := imaging.SHA256File(a)
	require.NoError(t, err)
	wantC, err := imaging.SHA256File(c)
	require.NoError(t, err)
	missing := filepath.Join(t.TempDir(), "missing.jpg")

	var lines []string
	ev := events.Events{OnLog: func(level events.Level, msg string) { lines = append(lines, level.String()+" "+msg) }}
	n, err := b.ProcessFiles([]string{a, missing, c}, "manual", ev)
	require.NoError(t, err)
	require.Equal(t, 2, n)

	require.NoFileExists(t, a)
	require.NoFileExists(t, c)
	require.True(t, b.IsBlacklisted(wantA))
	require.True(t, b.IsBlacklisted(wantC))
	items, err := b.Items()
	require.NoError(t, err)
	require.Len(t, items, 2)
	for _, it := range items {
		require.Equal(t, "manual", it.Source)
		require.NotNil(t, it.Thumbnail, "thumbnail must be captured before the file is deleted")
	}
	require.Equal(t, []string{
		"[INFO] Blacklisted and removed: a.jpg",
		"[WARN] File not found: " + missing,
		"[INFO] Blacklisted and removed: c.png",
	}, lines)
}

func TestProcessFilesKeepsFileWhenBlacklistWriteFails(t *testing.T) {
	b, err := OpenBlacklist(filepath.Join(t.TempDir(), "blacklist.db"))
	require.NoError(t, err)
	require.NoError(t, b.Close())
	a := copyFile(t, goldenImage(t, "small_800x600.jpg"), "a.jpg")
	n, err := b.ProcessFiles([]string{a}, "manual", events.Events{})
	require.Error(t, err)
	require.Zero(t, n)
	require.FileExists(t, a, "an image that was never recorded must not be deleted")
}
