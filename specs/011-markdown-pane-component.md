# Spec 011: Markdown Pane Component (About section scrolling)

> **Note**: This work has no associated issue tracker ticket. Personal public
> repository, no tracker (see `.claude/CLAUDE.md`).

## Status: COMPLETE

## Executive Summary

The About section scrolled in fits and bursts. Two causes: it rendered the
whole embedded README as a single Fyne RichText, which lays out and repaints
all ~250 of its segments on every refresh; and Fyne draws a Markdown code
block inside a horizontal scroll, which takes the wheel from the section and
spends it sideways, so the page stopped dead wherever the pointer rested on
one of the README's commands. This adds `markdownPane`, a shared component
that splits a Markdown document into blocks, measures each one once per width,
keeps only the blocks within half a viewport of the screen in the widget tree,
and draws code on a panel of its own that wraps instead of scrolling.
Reviewers should look at `internal/gui/markdownpane.go` (the component, its
height contract and `codePanel`) and at the design-language table in
`docs/architecture.md`, which is where the vocabulary of section components is
now written down.

## Context

`internal/gui/views_about.go` built the About section as
`RichText(README) inside the section scroll`. The README is 221 lines, which
Fyne parses into about 250 segments in one widget. Fyne re-lays-out and
repaints a RichText's segments on every refresh, and a scroll refreshes its
content as it moves, so the cost of the whole document was paid on every
wheel notch. Observed as stuttering, "fits and bursts" scrolling in the
About section on KDE Plasma 6.

The larger half of that is not cost at all. Since Fyne 2.8 a Markdown code
block is drawn as a label inside a horizontal scroll
(`widget.richCodeBlock`), and Fyne gives a wheel event to the innermost
scrollable under the pointer without passing it on. Worse, when the code is
wider than the pane and the block is not taller than it, `Scroll.scrollBy`
swaps the axes and turns a vertical notch into a sideways one. The README
shows every command as an indented block — 57 of its lines — so the page
stopped dead wherever the pointer happened to rest on one, which is what the
fits and bursts were.

Measured with the software painter over a 900x700 pane (the numbers are
relative, not absolute: the GL driver paints differently): a frame during a
scroll cost ~51 ms with one RichText and ~33 ms with the pane at half a
viewport of overscan, with 21 of 44 blocks rendered instead of all of them.

This is the first component in this window that exists for rendering cost
rather than for a shape. The section vocabulary it joins (`heading`, `card`,
`note`, `detailTable`, `logPane`) was until now documented only in the code,
so this spec also records it as a design language in `docs/architecture.md`.

## Requirements

- Nothing inside the document takes the wheel from the section that holds it.
- A Markdown document longer than the window renders only the part of itself
  near the viewport, and renders the rest as it is scrolled to.
- The document's height and every block's position are the same whether or
  not a block is currently rendered: scrolling never reflows the interface
  and the scrollbar never jumps (the project's "nothing transient may reflow
  the interface" rule).
- Block heights are measured, never estimated, and measured again when the
  width changes.
- The component is reusable and belongs to the window's design language, not
  to the About section; the About README is its first caller.
- The component releases the scroll it watches when its section is replaced.
- The design language (every shared section component and when to use it) is
  documented.

## Acceptance Criteria

- [x] AC1 — A document taller than the viewport keeps its distant blocks out of the widget tree; the first block renders and the last does not — `internal/gui/markdownpane.go` `sync`; `TestMarkdownPaneKeepsDistantBlocksOutOfTheWidgetTree`.
- [x] AC2 — Every block renders by the time it is scrolled to, and blocks left far behind are released — `TestMarkdownPaneRendersBlocksAsTheyAreScrolledTo`.
- [x] AC3 — The document's height does not change as blocks are rendered and released; each block keeps a spacer at its measured height — `markdownPane.render`; `TestMarkdownPaneHeightIsTheSameWhicheverBlocksAreRendered`.
- [x] AC4 — Narrowing the pane measures the blocks again, so a narrower window wraps into a taller document rather than clipping — `markdownPane.measure`; `TestMarkdownPaneMeasuresAgainWhenTheWindowNarrows`.
- [x] AC5 — `detach` gives the scroll back, so a replaced section stops rendering into a tree nobody is looking at; `ui.detach` calls it — `internal/gui/app.go` `detach`; `TestMarkdownPaneDetachStopsWatchingTheScroll`.
- [x] AC6 — The overscan margin is half a viewport either side, which measured about a third off the per-frame cost of the same document as one RichText — `mdOverscan` in `internal/gui/markdownpane.go`.
- [x] AC7 — A fenced code block containing a blank line stays one block, and the blocks together hold every non-blank line of the document in order — `markdownBlocks`; `TestMarkdownBlocksKeepFencedCodeWhole`, `TestMarkdownBlocksKeepEveryLineOfTheDocument`.
- [x] AC8 — A pane with no scroll to follow renders all of its blocks, so the component is usable outside a scrolling section — `TestMarkdownPaneWithNoViewportRendersEverything`.
- [x] AC9 — The About section uses the pane and holds it on `ui.readme` for the duration it is on screen — `internal/gui/views_about.go` `buildAbout`.
- [x] AC10 — `docs/architecture.md` documents the design language: every shared section component, when to use it, and the two rules (colour from the scheme, nothing transient reflows) — "Design language" under `internal/gui`.
- [x] AC11 — Nothing the pane renders is scrollable, so the wheel always belongs to the section: code blocks are drawn by this package's `codePanel` instead of by Fyne's scrolling one — `renderMarkdownBlock`, `codePanel`; `TestAboutDocumentHasNothingInItThatTakesTheWheel`, and `TestFyneStillDrawsMarkdownCodeInsideAScroll` as the canary for the upstream behaviour being fixed.
- [x] AC12 — Both Markdown code spellings are recognised, prose and list continuations are not, and a panel shows every line of its code (wrapped, not clipped) — `codeBlockText`; `TestCodeBlockTextStripsFencesAndIndents`, `TestCodePanelShowsEveryLineOfTheCode`.

- [x] AC13 — `make test` passes (`go test -race -tags parity ./...`) and `make lint` is clean.

## Risks & Assumptions

- **Rollback**: revert the commit and reinstall (`./install.sh`). GUI-only
  change, no on-disk format and no core operation touched, so nothing to
  migrate either way. Per `release-safety/simplified.md`.
- **CLI/GUI parity**: unaffected. The pane is presentation; no user-facing
  operation was added, so `internal/core` and `tests/parity` are untouched.
- **Assumption**: blank-line splitting is the right block boundary for the
  documents this window shows. It is Markdown's own paragraph rule; fenced
  code is handled explicitly, and a document that relied on a single RichText
  spanning a blank line (a lazy-continuation list) would render as separate
  blocks. The README does not.
- **Assumption**: measuring every block at build time (one parse of the
  document, as before) is acceptable; only painting is made lazy, not
  parsing. Measured at a few milliseconds for the README.
- **Deliberate deviation**: a code panel wraps long lines rather than
  scrolling them sideways as Fyne does. Sideways scrolling reads better but
  hides the rest of the line behind the gesture the page itself needs.
- **Known limitation**: the README's one image (`img/co_gui_example.png`) is
  a relative path that Fyne cannot resolve at run time. That was true of the
  previous rendering too and is out of scope here.
- **Verification gap**: the cost measurement was taken with the software
  painter in a headless test; the GL driver's per-frame cost was not measured
  directly. The wheel fix is verified by test (nothing the pane renders is
  scrollable) and on the desktop by scrolling the About section with the
  pointer over a command block.
