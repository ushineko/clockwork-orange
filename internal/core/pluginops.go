package core

import (
	"context"
	"fmt"
	"maps"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/plugins"
)

// RunPluginRequest runs one plugin by name (R6.4, `--plugin`, `plugin run`).
type RunPluginRequest struct {
	Request
	Name string
	// Override is merged over the plugin's config block (`--plugin-config`).
	Override map[string]any
	// Force bypasses the interval; Reset wipes the download dir first.
	Force, Reset bool
	// Doc, when non-zero, is used instead of loading the config.
	Doc *config.Document
}

// RunPlugin executes the plugin and returns its result.
func RunPlugin(ctx context.Context, req RunPluginRequest) (plugins.Result, error) {
	d := req.deps()
	if err := d.ensureRegistry(); err != nil {
		return plugins.Result{}, err
	}
	p, ok := plugins.Lookup(d.Registry, req.Name)
	if !ok {
		return plugins.Result{}, fmt.Errorf("unknown plugin %q (available: %v)", req.Name, plugins.Names(d.Registry))
	}
	doc := config.Document{}
	if req.Doc != nil {
		doc = *req.Doc
	} else {
		loaded, err := LoadConfig(ctx, req.Request)
		if err != nil {
			return plugins.Result{}, err
		}
		doc = loaded.Doc
	}
	cfg := map[string]any{}
	maps.Copy(cfg, doc.Plugins[req.Name])
	maps.Copy(cfg, req.Override)
	if req.Force {
		cfg["force"] = true
	}
	if req.Reset {
		cfg["reset"] = true
	}
	return p.Run(ctx, cfg, req.Events)
}

// PluginInfo describes one available plugin for `plugins list` and the GUI.
type PluginInfo struct {
	Name        string
	Description string
	Schema      []plugins.Field
	Enabled     bool
	// Config is the plugin's block from the loaded document (may be nil).
	Config map[string]any
}

// PluginsList returns the registry in order with each plugin's enablement.
func PluginsList(ctx context.Context, req Request) ([]PluginInfo, error) {
	d := req.deps()
	if err := d.ensureRegistry(); err != nil {
		return nil, err
	}
	loaded, err := LoadConfig(ctx, req)
	if err != nil {
		return nil, err
	}
	out := make([]PluginInfo, 0, len(d.Registry))
	for _, p := range d.Registry {
		out = append(out, PluginInfo{
			Name:        p.Name(),
			Description: p.Description(),
			Schema:      p.Schema(),
			Enabled:     loaded.Doc.PluginEnabled(p.Name()),
			Config:      loaded.Doc.Plugins[p.Name()],
		})
	}
	return out, nil
}

// PluginNames lists the registry names without touching the config or the
// stores' contents (used to build `--plugin` choices).
func PluginNames(req Request) ([]string, error) {
	d := req.deps()
	if err := d.ensureRegistry(); err != nil {
		return nil, err
	}
	return plugins.Names(d.Registry), nil
}
