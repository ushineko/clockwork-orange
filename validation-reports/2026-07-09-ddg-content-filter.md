## Validation Report: DuckDuckGo plugin content filtering

**Date**: 2026-07-09
**Commit**: (pre-commit)
**Status**: PASSED
**Spec**: none (bug fix — no dedicated spec file)

### Problem

The DuckDuckGo Images plugin downloaded and saved non-wallpaper content (ads,
portrait photos of people, square product/toy shots). Root causes:

1. The only content gate was a ≥1920×1080 resolution check; ads and product
   photos routinely clear it.
2. `_resize_and_crop` force-fills *any* aspect ratio into 16:9, so portrait and
   square sources were reshaped into wallpapers and saved rather than rejected.
3. No content-type/shape filter was requested from DuckDuckGo; the `ddgs` path
   set no safe-search at all, and pulled `max_results=200`, deep into the
   low-relevance tail where clean queries bleed into tangential images.
4. Image downloads carried no `Referer`, so hosts with hotlink protection
   returned ad/placeholder substitutes instead of the indexed image.

### Changes

- Added a landscape/aspect shape gate (`_is_wallpaper_shaped`, band 1.2–2.5)
  applied at discovery (`_filter_results`, when dims are present) and as the
  authoritative check on the decoded download (`_process_image`).
- `ddgs` path now passes `safesearch="on"`, `type_image="photo"`,
  `layout="Wide"`, and `max_results=60`.
- Direct scrape path `f` param now `type:photo,size:Large,layout:Wide`; result
  set capped to 60.
- `_filter_results` carries each result's source-page URL; `_process_image`
  sends it as `Referer` on the image download.

### Phase 3: Tests

- New `tests/test_duckduckgo_filter.py` (7 cases): shape gate accept/reject,
  discovery filter (resolution + aspect + dedup + no-dims passthrough), Referer
  derivation, Referer sent on download, non-landscape decoded download rejected
  before save. All pass.
- Full suite: `python -m pytest tests/` → 19 passed, 2 skipped (platform-gated
  frozen-import tests), 0 failures.
- `python -m py_compile plugins/duckduckgo_images.py` clean.
- Status: PASSED

### Phase 4: Code Quality

- Dead code: none introduced; the previous inline `min_w, min_h = 1920, 1080`
  literals replaced by module constants reused across both gates.
- Duplication: shape check centralised in one static helper used at both the
  discovery and download stages rather than duplicated.
- Encapsulation: no signature churn beyond the additive `referer` parameter and
  the candidate-dict return shape; both scrape paths converge on the same shape.
- Status: PASSED

### Phase 5.5: Release Safety (simplified — desktop app)

- Rollback: revert the commit / reinstall previous release. No schema, no
  migration, no persisted-state change. History/blacklist DB untouched.
- Additive change: stricter filtering only reduces what is downloaded; existing
  wallpapers on disk are not touched or removed by this change.
- Behavioural note: stricter filters mean fewer images accepted per run; this is
  the intended effect. `max_results` lowered 200→60 reduces bandwidth.
- Status: PASSED

### Security (Phase 5)

- No new dependencies. No secrets added or logged.
- New outbound header: the DuckDuckGo-supplied source-page URL is sent as
  `Referer` to the image host. This is standard browser behaviour and exposes no
  local/credential data. Image URLs remain DuckDuckGo-sourced as before.
- `pip-audit` not run in this session (no dependency changes to scan); code
  change is filter logic only.
