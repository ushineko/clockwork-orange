# Project-Specific Guidelines: clockwork-orange

This file extends the global Ralph methodology (`~/.claude/CLAUDE.md`).

---

## Selected Policies

- `git/standard.md`
- `release-safety/simplified.md`

**Not selected**: `languages/python.md` — this project's Python conventions differ from the global policy. See [Python Conventions (Project Override)](#python-conventions-project-override) below.

---

## Project Overview

- **Type**: Desktop application (GUI + CLI)
- **Language**: Python 3.10+
- **Platforms**: KDE Plasma 6 (Linux), Windows 10/11, macOS 13+
- **Framework**: PyQt6 for GUI
- **Purpose**: Wallpaper and lock screen management with plugin system

---

## Relaxed Rules

### Release Safety: Simplified

**Justification**: This is a desktop application with no backend databases, APIs, or infrastructure. Schema migrations and multi-phase rollouts do not apply.

**What this means**:
- Document rollback approach (revert commit, reinstall previous version)
- Skip formal Expand-Migrate-Contract checklists
- Skip ringed rollout planning
- Focus on: Can users easily revert to previous version if needed?
- For releases: Users reinstall previous version from GitHub Releases

---

## Python Conventions (Project Override)

This section **overrides** the global `languages/python.md` policy. The global policy targets a different project profile (>=3.12, uv, pyproject.toml, Pydantic). This project's conventions:

### Targets and Tooling
- **Python**: 3.10+ (must run on all supported platforms)
- **Packaging**: `pip` with `requirements.txt` (no uv, no pyproject.toml)
- **Build**: PyInstaller for frozen executables
- **Testing**: `pytest`
- **No Pydantic**: Configuration is YAML-based via PyYAML

### Style
- Follow PEP 8
- Use type hints for function signatures where practical
- Prefer `list`, `dict`, `tuple` over `typing.List`, `typing.Dict`, `typing.Tuple`
- Keep functions focused; extract helpers when patterns repeat

### Platform Awareness
- Conditional imports for platform-specific modules (`pywin32`, `AppKit`)
- Use `platform_utils.py` as the abstraction layer for OS differences
- Guard platform-specific code paths with runtime checks, not import-time failures

---

## Additional Rules

### Platform-Specific Testing

When making changes:
- Test on Windows if modifying `platform_utils.py` or Windows-specific code
- Test GUI changes with PyQt6
- Run `--self-test` on frozen executables after build changes

### Build Verification

Before releasing Windows builds:
1. Run `.\scripts\build_windows.ps1` (debug build)
2. Execute `.\dist\clockwork-orange.exe --self-test`
3. Verify all imports pass, especially platform-specific ones

---

## Release Workflow

When the user says **"release"** or **"releasing"**, follow this workflow:

### Step 1: Determine Version Bump

Use semantic versioning (semver) to determine the new version:

| Change Type | Version Component | Example |
|-------------|-------------------|---------|
| Bugfix / minor patch | Patch (x.y.**Z**) | v2.7.2 → v2.7.3 |
| New feature | Minor (x.**Y**.0) | v2.7.2 → v2.8.0 |
| Major/breaking change | Major (**X**.0.0) | v2.7.2 → v3.0.0 |

### Step 2: Confirm Version with User

**Before updating `.tag`**, use `AskUserQuestion` to confirm:

```
Current version: v2.7.2
Suggested new version: v2.7.3 (patch bump for bugfix)

Options:
1. Accept v2.7.3
2. Use minor bump (v2.8.0)
3. Use major bump (v3.0.0)
4. Other (specify custom version)
```

### Step 3: Update .tag File

Update the `.tag` file with the confirmed version:
```
v2.7.3
```

### Step 4: Run Release Script

**On Linux/macOS:**
```bash
./release_version.sh
```

**On Windows (via Git Bash or MSYS2):**
```bash
bash release_version.sh
```

The script will:
1. Commit `.tag` if modified
2. Create annotated git tag
3. Push to remote (main branch + tag)

### Step 5: Verify Release

After the script completes:
1. Confirm tag appears on GitHub
2. GitHub Actions will build Windows executable automatically
3. Check Actions workflow for build success

### Release Checklist

- [ ] All tests passing
- [ ] Self-test passes on frozen executable (if Windows changes)
- [ ] Version confirmed with user
- [ ] `.tag` updated
- [ ] `release_version.sh` executed successfully
- [ ] Tag visible on GitHub
- [ ] GitHub Actions build successful

---

## Environment

- **Python**: 3.10+ (3.12 for development)
- **Package Manager**: pip (from requirements.txt)
- **Build Tool**: PyInstaller for Windows executables
- **GUI Framework**: PyQt6

### Key Dependencies
- PyQt6 (GUI)
- Pillow (image processing)
- requests (network)
- PyYAML (configuration)
- watchdog (file system monitoring)
- pywin32 (Windows-specific features)

---

## Security Extensions

### Additional Checks for This Project

- **No network credentials**: API keys (Wallhaven, etc.) should use config files, not hardcoded
- **Safe file operations**: Validate paths for wallpaper sources, avoid path traversal
- **Plugin security**: Plugins should not execute arbitrary code from network sources

---

## Configuration Summary

| Category | Setting | Policy Module | Notes |
|----------|---------|---------------|-------|
| Git | Standard | `git/standard.md` | Conventional commits, connectivity checks, no co-authored-by |
| Release Safety | Simplified | `release-safety/simplified.md` | Desktop app, no backend infrastructure |
| Python | Project override | *(inline above)* | 3.10+, pip, requirements.txt |
| Validation Reports | Strict | *(core methodology)* | Required before every commit |
| Code Quality Checks | Strict | *(core methodology)* | Always check for dead code, duplication |
| Test Requirements | Strict | *(core methodology)* | Tests must pass |
| Communication Style | Strict | *(core methodology)* | Factual language, no superlatives |
| Tool Installation | Strict | *(core methodology)* | Always ask before installing |
| Security | Mandatory | *(core methodology)* | CVE scanning, OWASP checks |

---

*Updated 2026-02-26 — added Selected Policies, Python project override, macOS platform*
