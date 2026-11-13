package core

import (
	"context"
	"fmt"

	"github.com/ushineko/clockwork-orange/internal/models"
)

// ModelInfo describes one catalog model for `models list` and the GUI.
type ModelInfo struct {
	Name        string
	Description string
	License     string
	Gated       bool
	SizeBytes   int64
	// Pulled is true when the file is on disk at its full size.
	Pulled bool
	// Path is where the file lives, or would live once pulled.
	Path string
}

// ListModelsRequest lists the Stable Diffusion models (spec 031 R6, R7).
type ListModelsRequest struct{ Request }

// ListModels returns the catalog with each model's pulled state.
func ListModels(_ context.Context, req ListModelsRequest) ([]ModelInfo, error) {
	st := req.deps().modelStore()
	out := make([]ModelInfo, 0, len(models.Catalog()))
	for _, m := range models.Catalog() {
		out = append(out, ModelInfo{
			Name: m.Name, Description: m.Description, License: m.License, Gated: m.Gated,
			SizeBytes: m.File.Size, Pulled: st.Pulled(m), Path: st.Path(m),
		})
	}
	return out, nil
}

// PullModelRequest downloads one catalog model.
type PullModelRequest struct {
	Request
	Name string
}

// PullModelResult is the model's name and where it now lives.
type PullModelResult struct {
	Name string
	Path string
}

// PullModel downloads a model, resuming a partial file and verifying its
// SHA-256, and reports progress through req.Events. Cancel ctx to stop; the
// partial file is kept for the next pull.
func PullModel(ctx context.Context, req PullModelRequest) (PullModelResult, error) {
	m, ok := models.Lookup(req.Name)
	if !ok {
		return PullModelResult{}, fmt.Errorf("unknown model %q (available: %s)", req.Name, modelNames())
	}
	path, err := req.deps().modelStore().Pull(ctx, m, req.Events)
	if err != nil {
		return PullModelResult{}, err
	}
	return PullModelResult{Name: m.Name, Path: path}, nil
}

func modelNames() string {
	s := ""
	for i, m := range models.Catalog() {
		if i > 0 {
			s += ", "
		}
		s += m.Name
	}
	return s
}

// modelStore is the configured store, or the default one on the user's data
// directory with the shared HTTP client.
func (d *Deps) modelStore() models.Store {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.Models.Dir != "" {
		return d.Models
	}
	st := models.NewStore()
	if d.HTTP != nil {
		st.HTTP = d.HTTP
	}
	return st
}
