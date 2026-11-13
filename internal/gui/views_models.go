package gui

import (
	"context"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ushineko/clockwork-orange/internal/core"
	"github.com/ushineko/fynedesygn/widgets"
)

// stableDiffusionPlugin is the plugin whose section carries the Models card.
const stableDiffusionPlugin = "stable_diffusion"

// loadModels reads the catalog and what is on disk (spec 031 R7). Local stats
// only, but like every core call from the window it stays off the UI thread.
func (u *ui) loadModels() {
	if u.modelsOK {
		return
	}
	u.modelsOK = true
	run := func() {
		infos, err := core.ListModels(context.Background(), core.ListModelsRequest{Request: u.request()})
		if err != nil {
			fyne.Do(func() { u.sh.Report("Read the models", err) })
			return
		}
		fyne.Do(func() {
			u.models = infos
			u.sh.Refresh()
		})
	}
	if !u.sh.OnScreen() {
		run()
		return
	}
	go run()
}

// modelsCard lists the models with their size and licence and a Pull button
// for each one that is not on disk. A pull runs behind the shell's cancellable
// busy popup, which floats over the section, so the card never reflows.
func (u *ui) modelsCard() fyne.CanvasObject {
	if !u.modelsOK {
		u.loadModels()
	}
	body := container.NewVBox()
	if len(u.models) == 0 {
		body.Add(widgets.Dim("Reading the models…"))
	}
	var pulls []*widget.Button
	for _, m := range u.models {
		title := widget.NewLabelWithStyle(m.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		detail := widget.NewLabel(fmt.Sprintf("%s · %s · %s", humanSize(m.SizeBytes), m.License, m.Description))
		detail.Importance = widget.LowImportance
		detail.Truncation = fyne.TextTruncateEllipsis
		var action fyne.CanvasObject
		if m.Pulled {
			action = widgets.Dim("Pulled")
		} else {
			name := m.Name
			b := widget.NewButtonWithIcon("Pull", theme.DownloadIcon(), func() { u.pullModel(name) })
			pulls = append(pulls, b)
			action = b
		}
		body.Add(container.NewBorder(nil, nil, nil, action, container.NewVBox(title, detail)))
	}
	u.sh.Gate(pulls...)
	hint := widget.NewLabel("Models are downloaded to " + modelsDirHint(u.models) +
		" and checked against a published SHA-256. A cancelled pull resumes where it stopped.")
	hint.Importance = widget.LowImportance
	hint.Wrapping = fyne.TextWrapWord
	body.Add(hint)
	return widgets.Card("Models", body)
}

// pullModel downloads one model behind the cancellable busy popup.
func (u *ui) pullModel(name string) {
	u.sh.PerformCancellable("Pulling "+name+"…", func(ctx context.Context) error {
		res, err := core.PullModel(ctx, core.PullModelRequest{Request: u.request(), Name: name})
		if err != nil {
			return err
		}
		u.modelsOK = false
		fyne.Do(func() { u.sh.OK("Pulled " + res.Name) })
		return nil
	})
}

// humanSize renders a byte count the way the catalog quotes it: 4.3 GB.
func humanSize(n int64) string {
	const gb, mb = 1e9, 1e6
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1f GB", float64(n)/gb)
	case n >= mb:
		return fmt.Sprintf("%.0f MB", float64(n)/mb)
	}
	return fmt.Sprintf("%d B", n)
}

// modelsDirHint is the directory the first model lives in, or a generic
// phrase before the catalog has loaded.
func modelsDirHint(infos []core.ModelInfo) string {
	if len(infos) == 0 {
		return "your data directory"
	}
	p := infos[0].Path
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			p = p[:i] // drop the file name
			break
		}
	}
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[:i] // drop the model's own directory
		}
	}
	return p
}
