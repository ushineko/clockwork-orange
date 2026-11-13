package gui

import (
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/models"
)

// The Models card reads the catalog through core, and a pull marks the model
// as pulled the next time the card loads (spec 031 R7).
func TestModelsCardLoadsTheCatalogAndAPullMarksAModelPulled(t *testing.T) {
	u, _, _ := testUI(t)
	st := models.Store{Dir: t.TempDir()}
	u.deps.Models = st

	u.loadModels()
	require.Len(t, u.models, len(models.Catalog()))
	for _, m := range u.models {
		require.False(t, m.Pulled, m.Name)
		require.NotEmpty(t, m.License, m.Name)
	}
	require.NotNil(t, u.modelsCard(), "the card builds from the loaded list")

	// Put a file of the catalog size in place: a pull of a model already on
	// disk succeeds without touching the network, and the card then sees it.
	m, _ := models.Lookup("sd-1.5")
	path := st.Path(m)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(m.File.Size))
	require.NoError(t, f.Close())

	u.pullModel("sd-1.5")
	require.False(t, u.modelsOK, "a pull invalidates the list")
	u.loadModels()
	var pulled bool
	for _, info := range u.models {
		if info.Name == "sd-1.5" {
			pulled = info.Pulled
		}
	}
	require.True(t, pulled)
}

func TestHumanSizeQuotesTheCatalogWay(t *testing.T) {
	require.Equal(t, "4.3 GB", humanSize(4265146304))
	require.Equal(t, "17 MB", humanSize(17_000_000))
	require.Equal(t, "12 B", humanSize(12))
}

func TestOnlyTheStableDiffusionSectionCarriesTheModelsCard(t *testing.T) {
	u, _, _ := testUI(t)
	build := func(name string) fyne.CanvasObject {
		info, ok := u.pluginInfo(name)
		require.True(t, ok, name)
		return u.configBody(name, u.newPluginForm(info, nil, func(map[string]any) {}))
	}
	// The Models card sits above the Configuration card in a column; every
	// other plugin gets the Configuration card alone.
	col, ok := build(stableDiffusionPlugin).(*fyne.Container)
	require.True(t, ok)
	require.Len(t, col.Objects, 2, "Models card, then Configuration card")
	if other, ok := build("local").(*fyne.Container); ok {
		require.NotEqual(t, 2, len(other.Objects), "local has no Models card") //nolint:testifylint // asserting not-a-pair
	}
}
