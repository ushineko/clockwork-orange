# Spec 007: Configuration Profiles and Presets

**Status**: PENDING
**Priority**: Medium
**Estimated Complexity**: Medium

## Overview
Add support for multiple configuration profiles (presets) that users can switch between for different use cases (work mode, gaming mode, relaxation mode, etc.).

## Requirements

### Functional Requirements
1. Create/save/delete named configuration profiles
2. Switch between profiles via GUI or CLI
3. Each profile stores: enabled plugins, wait interval, dual-wallpaper mode, plugin-specific settings
4. Default profile for fallback
5. Import/export profiles for sharing

### Technical Requirements
1. Store profiles in `~/.config/clockwork-orange-profiles/`
2. Each profile is a separate YAML file
3. Active profile tracked in main config
4. GUI profile selector dropdown
5. Preserve blacklist across all profiles (shared)

## Implementation Details

### Profile Storage Structure
```
~/.config/clockwork-orange-profiles/
  ├── default.yml
  ├── work.yml
  ├── gaming.yml
  └── relaxation.yml
```

### Profile Format
```yaml
profile_name: "Work Mode"
created_at: "2026-02-01T10:00:00"
description: "Professional wallpapers for work hours"

dual_wallpapers: false
default_wait: 3600  # 1 hour

plugins:
  local:
    enabled: true
    path: "/home/user/Wallpapers/Professional"
    recursive: true

  google_images:
    enabled: false

  wallhaven:
    enabled: true
    query:
      - term: "minimalist office"
        enabled: true
    purity_nsfw: false
```

### Main Config Update
```yaml
# Main config tracks active profile
active_profile: "work"
```

### GUI Profile Manager
Add to Settings tab:
- **Profile Dropdown**: Select active profile
- **New Profile Button**: Create new profile from current settings
- **Rename Profile Button**: Rename selected profile
- **Delete Profile Button**: Remove profile (with confirmation)
- **Export Profile Button**: Save profile to file for sharing
- **Import Profile Button**: Load profile from file

### CLI Support
```bash
# List profiles
./clockwork-orange.py --list-profiles

# Switch profile
./clockwork-orange.py --profile work

# Create new profile from current config
./clockwork-orange.py --save-profile "Work Mode"

# Export profile
./clockwork-orange.py --export-profile work --output work-profile.yml

# Import profile
./clockwork-orange.py --import-profile work-profile.yml
```

## Acceptance Criteria

- [ ] Profile storage directory created on first use
- [ ] Create new profile from current configuration
- [ ] Switch between profiles and verify settings change
- [ ] Delete profile (with confirmation)
- [ ] Rename existing profile
- [ ] Export profile to YAML file
- [ ] Import profile from YAML file
- [ ] Profile dropdown in GUI Settings tab
- [ ] Active profile persists across restarts
- [ ] Service respects active profile settings
- [ ] Blacklist remains shared across all profiles
- [ ] Profile validation (reject invalid YAML)

## Testing

### Test Cases
1. Create 3 profiles with different settings
2. Switch between profiles and verify wallpaper behavior changes
3. Export profile and import on different machine
4. Delete profile while it's active (should fallback to default)
5. Rename active profile
6. Create profile with invalid YAML (error handling)
7. Switch profiles while service is running

### Expected Behavior
- Profile switches are immediate (no restart required)
- Service picks up profile changes automatically
- Blacklist applies to all profiles
- Deleted profiles don't leave orphaned config
- Invalid profiles are rejected with clear error messages

## Notes
- Profiles are independent configurations but share blacklist
- Default profile cannot be deleted (fallback)
- Profile names must be filesystem-safe (no special characters)
- Future: Schedule profiles (e.g., work mode 9-5, gaming mode evenings)
- Future: Quick-switch hotkey support
- Future: Profile templates (pre-configured profiles for common use cases)

## Open Questions
1. Should profiles have separate blacklists or share one global blacklist?
   - **Decision**: Share global blacklist (users don't want same image in different profiles)

2. Should profile switch require service restart?
   - **Decision**: No, watchdog monitors config changes and reloads

3. Should there be a "quick switch" GUI tray icon?
   - **Defer**: Nice-to-have for future iteration
