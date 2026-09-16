package plugins

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Live network tests (R9.3), gated by CLOCKWORK_LIVE_NET=1. They talk to the
// real services with no API key and SFW-only settings.

func requireLiveNet(t *testing.T) {
	t.Helper()
	if os.Getenv("CLOCKWORK_LIVE_NET") != "1" {
		t.Skip("set CLOCKWORK_LIVE_NET=1 to run live network tests")
	}
}

func TestLiveWallhavenSearchReturnsAtLeastOneItem(t *testing.T) {
	requireLiveNet(t)
	p := newWallhaven(Deps{}.withDefaults())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	items, err := p.searchAPI(ctx, buildAPIParams(map[string]any{
		"purity_sfw": true, "purity_sketchy": false, "purity_nsfw": false,
	}, "landscape"))
	require.NoError(t, err)
	require.NotEmpty(t, items)
	require.NotEmpty(t, items[0].Path)
	require.NotEqual(t, "None", items[0].idString())
}

func TestLiveDuckDuckGoVqdAndResultsRoundTripReturnsAtLeastOneURL(t *testing.T) {
	requireLiveNet(t)
	p := newDuckDuckGo(Deps{}.withDefaults())
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	rec := newRecorder()
	urls, err := p.scrapeDirect(ctx, "4k nature wallpapers", rec.events())
	require.NoError(t, err)
	require.NotEmpty(t, urls, "logs: %v", rec.logs)
}
