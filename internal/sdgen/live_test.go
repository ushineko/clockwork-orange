package sdgen

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLiveGenerate drives the real native library and a real checkpoint (spec
// 031 R5, the integration-boundary criterion). It is gated because it needs
// both: CLOCKWORK_LIVE_SD=1, CLOCKWORK_SD_LIB (a directory of stable-diffusion.cpp
// libraries) and CLOCKWORK_SD_MODEL (a checkpoint path).
func TestLiveGenerate(t *testing.T) {
	if os.Getenv("CLOCKWORK_LIVE_SD") != "1" {
		t.Skip("set CLOCKWORK_LIVE_SD=1, CLOCKWORK_SD_LIB and CLOCKWORK_SD_MODEL to run")
	}
	lib, model := os.Getenv("CLOCKWORK_SD_LIB"), os.Getenv("CLOCKWORK_SD_MODEL")
	require.NotEmpty(t, lib, "CLOCKWORK_SD_LIB")
	require.NotEmpty(t, model, "CLOCKWORK_SD_MODEL")

	var steps int
	sess, err := Open(Options{LibDir: lib, ModelPath: model, Device: -1,
		OnStep: func(step, _ int) { steps = max(steps, step) }})
	require.NoError(t, err)
	defer sess.Close()
	require.NotEmpty(t, LibraryVersion())

	img, err := sess.Generate(Request{Prompt: "a red apple on a table", Width: 256, Height: 256, Steps: 4, CFGScale: 7, Seed: 7})
	require.NoError(t, err)
	require.Equal(t, 256, img.Bounds().Dx())
	require.Positive(t, steps, "the library reported sampling progress")

	// A flat image means the model produced nothing: require some variation.
	first := img.At(0, 0)
	varied := false
	for y := 0; y < 256 && !varied; y += 16 {
		for x := 0; x < 256; x += 16 {
			if img.At(x, y) != first {
				varied = true
				break
			}
		}
	}
	require.True(t, varied, "the generated image is a single flat colour")
}
