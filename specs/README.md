# Clockwork Orange Specifications

This directory contains feature specifications for the Clockwork Orange wallpaper manager project.

## Spec Format

Each spec follows this structure:

### Header
- **Status**: COMPLETE | PENDING | IN_PROGRESS
- **Implementation Date**: When completed (if applicable)
- **Commit**: Reference commit(s) (if applicable)
- **Priority**: High | Medium | Low (for pending specs)
- **Estimated Complexity**: High | Medium | Low (for pending specs)

### Sections
1. **Overview**: Brief description of the feature
2. **Requirements**: Functional and technical requirements
3. **Implementation Details**: Technical approach and design
4. **Acceptance Criteria**: Checklist of completion criteria (use `[x]` when done)
5. **Testing**: Test cases and expected behavior
6. **Notes**: Additional context, future considerations, open questions

## Spec Numbering

Specs are numbered sequentially:
- `001-xxx.md` - First spec
- `002-xxx.md` - Second spec
- etc.

Higher numbers generally indicate later features, but this isn't strict.

## Completed Specs

These specs document features that have been implemented:

- **001**: Multi-Monitor Support - KDE Plasma 6 multi-monitor wallpaper handling
- **002**: Plugin Architecture - Extensible plugin system for wallpaper sources
- **003**: Qt GUI - PyQt6-based graphical user interface
- **004**: Windows Support - Windows 10/11 platform support with PyInstaller
- **005**: Blacklist System - Shared blacklist and image review functionality
- **006**: Service/Daemon Mode - Background service with systemd/Task Scheduler

## Using Specs with Ralph Loop Mode

When using `/ralph` or Ralph Loop Mode:

1. Ralph will scan the `specs/` directory
2. Identify incomplete specs (status != COMPLETE or unchecked criteria)
3. Work through specs sequentially (lowest number first)
4. Mark acceptance criteria as complete: `[ ]` → `[x]`
5. Update status from PENDING → IN_PROGRESS → COMPLETE
6. Commit changes when all criteria are met

## Creating New Specs

When adding new features:

1. Copy the template structure from existing specs
2. Use next sequential number: `008-feature-name.md`
3. Set status to PENDING
4. Include priority and complexity estimates
5. Write clear acceptance criteria (checkboxes)
6. Commit the spec before implementation

## Retroactive Specs

All current specs (001-006) were created retroactively to document existing features implemented in late 2025 and early 2026. This provides:
- Documentation of design decisions
- Reference for maintenance and enhancements
- Foundation for Ralph Loop workflow
- Historical context for new contributors
