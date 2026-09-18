## Validation Report: Spec 011 — Markdown Pane Component (About section scrolling)

**Date**: 2026-09-17 10:47
**Spec**: specs/011-markdown-pane-component.md
**Commit**: pre-commit (working tree vs 293b2d5)
**Status**: PASSED

### Summary

The About section scrolled in fits and bursts. Two causes, both addressed by a
new shared component, `markdownPane`:

1. The whole embedded README was one Fyne RichText (~250 segments), laid out
   and repainted on every refresh, and a scroll refreshes its content as it
   moves. The pane now splits the document on blank lines, measures each block
   once per width into a spacer, and keeps only the blocks within half a
   viewport of the screen in the widget tree.
2. Since Fyne 2.8 a Markdown code block is drawn as a label inside an HScroll
   (`widget/richtext_objects.go:476`). Fyne hands a wheel event to the
   innermost scrollable under the pointer and does not pass it on, and
   `Scroll.scrollBy` (`internal/widget/scroller.go:641`) swaps the axes when
   the content is wider than the view and not taller, so a vertical notch
   scrolled the code sideways instead of moving the page. The README shows
   every command as an indented block — 57 lines — so the page stopped dead
   over most of the document. Code blocks are now drawn by this package's own
   `codePanel`, which wraps instead of scrolling.

Also in the change set: the About/LICENSE copyright line moves to 2025-2026,
and `docs/architecture.md` gains a "Design language" section recording the
window's shared component vocabulary and the rule this bug exposed — one
scroll per section.

### Phase 3: Tests

- Test suite: `make test` (`go test -race -tags parity ./...`)
- Results: 10 packages ok, 0 failing. 12 new tests in `internal/gui`
  (markdownPane lifecycle and height contract, wheel ownership, code-block
  splitting, code panel content).
- Coverage: not measured (project does not gate on a coverage number)
- Lint: `make lint` (golangci-lint v2.12.2, pinned config) — 0 issues
- Status: PASSED

Behavioural contracts, not implementation contracts: the tests assert what the
reader sees — distant blocks are not in the tree, every block renders by the
time it is scrolled to, the document does not change height while scrolling,
narrowing re-measures, nothing rendered takes the wheel. One deliberate
canary, `TestFyneStillDrawsMarkdownCodeInsideAScroll`, pins the *upstream*
behaviour `codePanel` exists to avoid, so the workaround can be deleted when
Fyne changes it.

### Phase 4: Code Quality

- Dead code: none found. `views_about.go` no longer builds a RichText
  directly; no import or helper was left behind (verified by build + lint,
  which runs `unused`).
- Duplication: none in the component. In the test file, `scrollableIn` and
  `textIn` are two recursive walkers of the same widget tree; they were left
  separate because one answers a predicate and the other collects strings, and
  a shared generic walker would be harder to read than either.
- Encapsulation: `markdownpane.go` is 356 lines including doc comments; no
  function exceeds 40 lines. Responsibilities are split: `markdownBlocks`
  (splitting), `measure` (geometry), `sync`/`render` (what is in the tree),
  `renderMarkdownBlock`/`codePanel` (drawing).
- Refactorings: the pane's block cache changed from `[]*widget.RichText` to
  `[]fyne.CanvasObject` when code panels were introduced, so the pane no
  longer assumes every block is a RichText.
- Status: PASSED

### Phase 5: Security Review (via /ralph-security-review)

- Verdict: PASS
- Quoted summary:
  - Phase A — `govulncheck@v1.1.4` (DB 2026-09-15), 0 findings. Source mode
    (`./...`) could not run: the installed govulncheck is built against go1.26
    while the local toolchain is go1.27.1. Ran `-mode=binary` against
    `bin/clockwork-orange` and `bin/clockwork-orange-gui` instead — no
    vulnerabilities in either. Covers the shipped code paths, not unreachable
    module-graph advisories. Restoring source mode needs a govulncheck
    reinstall (not performed: tool installs require the user's approval).
  - Phase B — OWASP Top 10, AI-assisted best-effort, not compliance evidence:
    all ten categories clean. The diff is GUI rendering — string slicing,
    widget construction, one struct field, one detach call. No exec, SQL,
    network, filesystem writes or logging. The only parsed input is the README
    embedded at build time with `go:embed`; the pane never renders remote or
    user-authored Markdown.
  - Phase C — secrets scan, inline (no `git-secrets`/`trufflehog`/
    `detect-secrets` installed): 0 findings.
  - Phase D — advisory: clean, no dependency manifests (`go.mod`/`go.sum`
    untouched) and no agent-config paths in the diff.
- Status: PASSED

### Phase 5.5: Release Safety

- Change type: code-only (GUI presentation), plus documentation
- Blast radius: `internal/gui` only. No `internal/core` operation added or
  changed, so CLI/GUI parity is untouched and `tests/parity` is unaffected. No
  on-disk format touched: config YAML, `history.db` and `blacklist.db` are not
  read or written by this change, so 2.9.x/4.x compatibility is unaffected.
- Rollback plan: revert the commit and reinstall (`./install.sh`), or install
  the previous release. Nothing to migrate in either direction. Per
  `release-safety/simplified.md` — desktop app, rollback is a reinstall.
- Status: PASSED

### Overall

- All gates passed: YES
- Notes:
  - Verification gap: the per-frame cost improvement was measured with the
    software painter in a headless test (~51 ms → ~33 ms per frame over a
    900x700 pane, 21 of 44 blocks rendered). The GL driver's cost was not
    measured directly. The wheel fix is verified by test — nothing the pane
    renders implements `fyne.Scrollable` — and on the desktop by scrolling the
    About section with the pointer over a command block.
  - Deliberate deviation recorded in the spec: a code panel wraps long lines
    rather than scrolling them sideways as Fyne does.
  - Out of scope, pre-existing: the README's one image reference
    (`img/co_gui_example.png`) is a relative path Fyne cannot resolve at run
    time, so the About section shows no picture. Unchanged by this work.
