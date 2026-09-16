package gui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/store"
)

// The filter matches any column, case-insensitively, and keeps the store's
// order (R7.8).
func TestBlacklistFilterMatchesAnyColumnCaseInsensitively(t *testing.T) {
	items := []store.BlacklistItem{
		{Hash: "AAAA1111", Date: "2026-09-15 10:00", Source: "wallhaven"},
		{Hash: "bbbb2222", Date: "2026-08-01 09:00", Source: "duckduckgo_images"},
		{Hash: "cccc3333", Date: "2025-12-24 18:00", Source: "local"},
	}
	require.Len(t, filterBlacklist(items, ""), 3)
	require.Len(t, filterBlacklist(items, "   "), 3)
	require.Equal(t, []store.BlacklistItem{items[0]}, filterBlacklist(items, "aaaa"))
	require.Equal(t, []store.BlacklistItem{items[1]}, filterBlacklist(items, "DUCK"))
	require.Equal(t, []store.BlacklistItem{items[0], items[1]}, filterBlacklist(items, "2026-"))
	require.Empty(t, filterBlacklist(items, "zzz"))
}

// A thumbnail table gets an image column and taller rows; a plain one does
// not pay for them.
func TestDetailTableThumbnailColumn(t *testing.T) {
	plain := &detailTable{}
	plain.header("A", "B")
	plain.row(StatusInfo, "1", "2")
	require.False(t, plain.hasThumbs())
	require.NotPanics(t, func() { plain.widget() })

	withThumbs := &detailTable{thumbCol: 1}
	withThumbs.header("", "Thumb", "Hash")
	withThumbs.row(StatusInfo, " ", "", "abc")
	withThumbs.thumbs = [][]byte{{0xff, 0xd8}}
	require.True(t, withThumbs.hasThumbs())
	require.NotNil(t, withThumbs.thumb(0))
	require.Nil(t, withThumbs.thumb(5))
	require.NotPanics(t, func() { withThumbs.widget() })
}
