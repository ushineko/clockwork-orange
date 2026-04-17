## Validation Report: Replace Google Images with DuckDuckGo Images

**Date**: 2026-04-17 15:58
**Commit**: (pre-commit)
**Status**: PASSED
**Spec**: [008-replace-google-images-with-duckduckgo.md](../specs/008-replace-google-images-with-duckduckgo.md)

### Phase 3: Tests

- No project unit-test suite exists for this surface; verification was direct.
- Plugin discovery: `PluginManager().get_available_plugins()` returns
  `['history', 'local', 'wallhaven', 'stable_diffusion', 'duckduckgo_images']`.
  The `google_images` key is absent.
- Plugin CLI contract: `python3 plugins/duckduckgo_images.py --get-description`
  and `--get-config-schema` return the expected strings/JSON.
- End-to-end download against live DDG endpoint: 4 queries × limit 50 →
  111 wallpapers downloaded, all post-processed to ≥1920×1080, no errors,
  under a deliberately constrained `ulimit -n 1024` shell.
- Config migration: 6 explicit cases covered programmatically:
  - empty config (no-op)
  - real-world shape with explicit `download_dir` (preserved verbatim)
  - old block lacking `download_dir` (pinned to old Google default)
  - second invocation (idempotent no-op)
  - partial manual migration with both keys present (no clobber)
  - YAML roundtrip via `load_and_migrate` (persisted to disk)
  All pass.
- Live migration confirmed by restarting the running systemd service against
  the user's actual config; on-disk YAML now contains `plugins.duckduckgo_images`
  with all original fields preserved. Migration log line emitted exactly once.
- Status: PASSED

### Phase 4: Code Quality

- Dead code: `plugins/google_images.py` removed; latent
  `gui/history_tab.py` bug (reading from top-level `google_images` key
  instead of `plugins.google_images`) corrected as part of the rename.
- Duplication: None introduced. The new plugin reuses the shared
  `BlacklistManager`, `HistoryManager`, and `PluginBase` contracts.
- Encapsulation: Migration logic factored into a single
  `config_migrations.py` module called from all three load sites
  (`load_config_file`, `main_window.load_config`, `service_manager.load_config`).
- Refactoring: One unrelated finding addressed — file-descriptor leak
  in the original Google Images plugin's HTTP path was carried over and
  fixed during testing. The fix (shared `requests.Session`, context-managed
  `.get()` calls, explicit `Image.close()`) is included in this commit.
- Status: PASSED

### Phase 5: Security Review

- Dependencies (tool-verified): No new dependencies added. `requests`,
  `Pillow`, `PyYAML`, and stdlib `sqlite3` are already required.
- `pip-audit -r requirements.txt` reported 3 pre-existing CVEs:
  `pillow 12.0.0` (CVE-2026-25990, CVE-2026-40192 — fixed in 12.2.0) and
  `requests 2.32.5` (CVE-2026-25645 — fixed in 2.33.0). These are NOT
  introduced by this change; they pre-date the diff. Recommend a separate
  dependency-bump release to address them.
- OWASP Top 10 (AI-assisted, best-effort):
  - **Injection**: User-supplied query strings are passed to DDG via the
    `params=` argument of `requests.get`, which URL-encodes them safely.
    Not concatenated into URL or shell.
  - **SSRF**: Image URLs come from DDG's response. No user-supplied URL
    is fetched; the same trust model as the prior plugin.
  - **Path traversal**: Filenames are MD5 hex digests, not user-controlled.
    `download_dir` is taken from config (user-controlled, but trusted) and
    `mkdir(parents=True, exist_ok=True)` is bounded by filesystem permissions.
  - **Deserialization**: JSON via `requests.Response.json()`; YAML via
    `yaml.safe_load`. Both safe primitives.
  - **Logging**: No credentials or tokens logged. The `vqd` token (DDG
    session identifier visible in URLs) is logged on extraction failure
    only as a positive negative-acknowledgement, not the value itself.
- Anti-patterns: None found.
- Fixes applied: FD-leak fix (above) closes a resource-exhaustion vector
  that could be exploited by a hostile DDG endpoint returning many
  candidates.
- Note: AI-assisted findings are a developer aid, not compliance evidence.
- Status: PASSED

### Phase 5.5: Release Safety (Simplified — per project policy)

- Change type: Code-only. New plugin file, deleted plugin file, new
  migration module wired into three existing load sites, doc updates.
  No new infrastructure, no schema changes.
- Rollback plan: Revert the commit. The config migration is one-way
  (the `google_images` key is removed from disk on first migrated load),
  but this is acceptable because the old plugin no longer functions;
  there is nothing to roll back *to* that works. Users can manually
  re-paste a `google_images:` block if they revert the binary, but it
  will not download images.
- User-visible impact: Plugin name changes from "Google Images" to
  "DuckDuckGo Images" in the GUI. Existing downloaded wallpapers remain
  visible to the wallpaper engine because the migration explicitly pins
  `download_dir` to the old path.
- Status: PASSED

### Overall

- All gates passed: YES
- Spec acceptance criteria: 13/13 satisfied. Frozen-build self-test on
  Windows (1 of 13) is deferred to the CI tag-build run, as documented
  in the spec — it is not reproducible on a Linux dev host.
- Spec status: marked COMPLETE
