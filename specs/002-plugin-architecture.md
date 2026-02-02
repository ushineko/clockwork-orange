# Spec 002: Plugin Architecture for Wallpaper Sources

**Status**: COMPLETE
**Implementation Date**: 2025-12
**Commit**: Multiple commits, Stable Diffusion added 2026-01-17

## Overview
Design and implement an extensible plugin system to support multiple wallpaper sources (local files, web APIs, AI generation) with a unified interface.

## Requirements

### Functional Requirements
1. Define a base plugin interface/abstract class
2. Support dynamic plugin loading and discovery
3. Allow multiple plugins to be enabled simultaneously
4. Implement fair source selection (prevent large local libraries from dominating)
5. Share blacklist database across all plugins

### Plugin Types
1. **Local Plugin**: Scan local directories for image files
2. **Wallhaven Plugin**: Download from Wallhaven.cc API
3. **Google Images Plugin**: Scrape and download from Google Images
4. **Stable Diffusion Plugin**: Generate AI wallpapers locally

### Technical Requirements
1. Each plugin must implement `BasePlugin` interface
2. Plugins must provide metadata (name, enabled status, configuration schema)
3. Support plugin-specific configuration in YAML
4. Plugin manager orchestrates execution and source aggregation

## Implementation Details

### Base Plugin Interface
```python
class BasePlugin:
    - get_name() -> str
    - is_enabled() -> bool
    - get_images() -> List[str]
    - get_config_schema() -> dict
    - execute() -> None
```

### Plugin Manager
- Load plugins from `plugins/` directory
- Filter enabled plugins based on configuration
- Aggregate images from all active plugins
- Implement fair random selection (source-first, then image)

### Configuration Format
```yaml
plugins:
  local:
    enabled: true
    path: "/path/to/wallpapers"
    recursive: true

  google_images:
    enabled: true
    query:
      - term: "4k nature wallpapers"
        enabled: true
```

## Acceptance Criteria

- [x] `BasePlugin` abstract class defined in `plugins/base.py`
- [x] `PluginManager` class handles plugin discovery and execution
- [x] Local plugin implemented and functional
- [x] Wallhaven plugin implemented with API integration
- [x] Google Images plugin implemented with scraping
- [x] Stable Diffusion plugin implemented with local generation
- [x] Multiple plugins can run concurrently
- [x] Fair selection algorithm ensures equal representation
- [x] Blacklist shared across all plugins
- [x] Plugin configuration persists in YAML

## Testing

### Test Cases
1. Single plugin enabled
2. Multiple plugins enabled simultaneously
3. Plugin with no images (graceful handling)
4. Plugin configuration validation
5. Fair selection with unequal source sizes (10 vs 10,000 images)

### Expected Behavior
- Plugins load without errors
- Each enabled plugin contributes to image pool
- Fair selection prevents large sources from dominating
- Blacklist applies to all plugin sources

## Notes
- Plugin isolation: Each plugin runs independently
- Error handling: Plugin failures should not crash main application
- Future: Support for third-party plugin installation
- Future: Plugin dependency management (e.g., Stable Diffusion requirements)
