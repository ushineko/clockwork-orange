//go:build unix

package gui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A second launch brings a hidden first window back instead of doing
// nothing (R7.19); with no first instance the request reports so.
func TestSecondLaunchShowsTheHiddenWindow(t *testing.T) {
	u, _, _ := testUI(t)
	require.False(t, requestShow(), "no listener yet")

	shown := make(chan struct{}, 4)
	u.showHook = func() { shown <- struct{}{} }
	stop := u.listenShow()
	defer stop()
	require.True(t, requestShow())
	select {
	case <-shown:
	case <-time.After(3 * time.Second):
		t.Fatal("the first instance did not act on the request")
	}

	stop()
	require.False(t, requestShow(), "the socket is gone once the instance stops")
}
