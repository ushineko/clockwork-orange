# Spec 019: Trim flat bars, and say what an API failure means

**Issues**: #21, #22

## Status: COMPLETE

- **Priority**: Medium
- **Estimated Complexity**: Medium
- **Branch**: `feat/trim-bars`

## Executive Summary

Two things found in one download run.

Some downloaded wallpapers arrive at the right resolution with flat padding
baked in — white down the sides, black across the top — and the cover-crop
preserves it, so it reaches the desktop. `imaging.TrimBars` removes it before
the crop. The detection is easy; doing it *safely* is the work, and three
guardrails take a naive detector from 70 false-positive hits on the author's
638 wallpapers down to 8 real ones.

Separately, a Wallhaven outage produced six copies of a Cloudflare error page
in the log and no explanation. The plugin now names what a status means, stops
the run when the API itself is unreachable, and truncates a body it does not
recognise.

## Context

### The bars (#21)

The reported example, measured:

    size 3840x2160
    uniform bright bars: left=191 right=187 top=0 bottom=0
    content box: 3462x2160  aspect 1.6028 (16:9 = 1.7778)
      col 0..140: mean=255.0  maxdev=0.0
      col 200:    mean=108.3  maxdev=134.4

Pure white, zero deviation, ending at a hard step. The detection is exact.

**The danger is the false positive.** A naive scan — walk inward while the line
is flat and extreme — claimed bars on **70 of 638** real wallpapers, including
`L2736 R2736` on a 5760-pixel-wide picture. It had marched through the flat
regions of dark space scenes and minimalist designs. Cropping on that would
have destroyed one wallpaper in ten to fix one in seventy.

### The outage (#22)

Measured live while the report came in:

| Endpoint | Result |
|---|---|
| `wallhaven.cc/` | 200 |
| `wallhaven.cc/api/v1/search` | 503, with our UA and a browser UA alike |
| `w.wallhaven.cc` | 521 |
| the same API through a third-party proxy, different source IP | 522 |

521 and 522 mean Cloudflare cannot reach the origin. A different IP fails
identically and a browser User-Agent gets the same 503, so it is not a bot
challenge, not an IP block, and not the missing API key — `purity=100` is
SFW-only, which the API serves keyless. The request was correct; what was wrong
was everything around it.

## Requirements

### R1. Trimming is conservative by construction

- R1.1 `imaging.TrimBars(src)` returns the content rectangle and whether
  anything was trimmed. Every guardrail that fails returns the original
  bounds: a picture it cannot confidently read is a picture it leaves alone.
- R1.2 A line is padding when it is flat (max deviation < 12) and extreme
  (mean > 225 or < 24), which is loose enough for JPEG ringing against a hard
  edge and nowhere near loose enough to reach content.
- R1.3 **Guardrail — width.** A bar may not exceed a quarter of its dimension.
  Padding to a common aspect ratio never needs more: 16:9 from 4:3 is 12.5% a
  side, from 1:1 it is 21.9%.
- R1.4 **Guardrail — the step.** A bar must end at a luma jump greater than 40.
  Padding meets content at an edge; a flat region of content fades into its
  neighbours.
- R1.5 **Guardrail — what survives.** More than half of each dimension must
  remain. Both sides at the cap leaves exactly half, which is refused.
- R1.6 **Guardrail — the floor.** A run under 8 pixels is a scaling artefact,
  not a letterbox. Removing it changes nothing visible and, for a plugin that
  would have to re-encode to do it, is a rewrite for no reason.

### R2. Applied to new downloads only

- R2.1 **DuckDuckGo**: trimmed before `CoverResizeCrop`, which is where the
  padding was being preserved. Free — the image is already decoded and about
  to be re-encoded.
- R2.2 **Wallhaven**: this plugin writes the bytes it downloaded rather than
  re-encoding them, and that is deliberate. `imaging.TrimBarsFile` decodes to
  look, and rewrites **only** the one image in seventy that is padded; the
  rest keep the bytes the source sent.
- R2.3 A rewrite keeps the file's existing format, so a PNG stays a PNG and
  nothing acquires JPEG artefacts it did not have. A format this package
  cannot write back is left alone.
- R2.4 Trimming happens before the hash is taken, so the history and blacklist
  record the picture that is on disk.
- R2.5 A failure to read a file back is logged at debug and does not discard
  the download.
- R2.6 **Nothing already on disk is touched.** A false positive on an existing
  file is unrecoverable, and 1.3% does not justify it. An image that annoys can
  still be blacklisted from the review.

### R3. An API failure says what it means

- R3.1 A recognised status is reported by meaning, not by body: 401 is a
  rejected key, 429 is a rate limit with the actual limit named, and
  503/521/522/523/524 are an outage at wallhaven.cc rather than a problem with
  the user's configuration.
- R3.2 An unrecognised status still shows its body, collapsed to one line and
  truncated to 200 characters.
- R3.3 When the API itself is unreachable the run stops and says so once,
  rather than putting the same error against every configured term.
- R3.4 A status that means *this query* was wrong does not stop the run.

### R4. Guards

- R4.1 Tests for pillarboxing, letterboxing, all four sides, and the exact
  geometry of the reported example.
- R4.2 Tests for each guardrail, using the shapes that produced the naive
  detector's false positives.
- R4.3 A test that a Cloudflare error page does not reach the log, that the
  message stays under 200 characters, and that each named status is named.
- R4.4 A test that only the outage statuses stop the run.
- R4.5 The detector is checked against the real 638-image corpus, not only
  against fixtures.

## Acceptance Criteria

- [x] AC1 The reported image's bars are found exactly (R1.1)
  - Verified: `TestTrimBarsRemovesPillarboxing` — 3840×2160 → 3462×2160
- [x] AC2 Letterboxing and black bars are handled (R1.1)
  - Verified: `TestTrimBarsRemovesLetterboxing`, `TestTrimBarsHandlesAllFourSides`
- [x] AC3 A picture with no padding is untouched, including the shapes that fooled the naive detector (R1.3, R1.4)
  - Verified: `TestTrimBarsLeavesAPictureAlone` — a gradient, a dark scene that fades into its subject, a minimalist picture on a white field
- [x] AC4 A trim that would take half the picture is refused (R1.5)
  - Verified: `TestTrimBarsRefusesToCropMostOfThePicture`
- [x] AC5 A few pixels of flat edge is ignored (R1.6)
  - Verified: `TestTrimBarsIgnoresAFewPixelsOfEdge`
- [x] AC6 Both plugins trim new downloads, and Wallhaven only rewrites what is padded (R2.1, R2.2)
  - Verified: `duckduckgo.go` before `CoverResizeCrop`; `wallhaven.go` via `TrimBarsFile` after the write and before the hash
- [x] AC7 Nothing already on disk is modified (R2.6)
  - Verified: no path walks an existing directory; both call sites are in the download pipeline
- [x] AC8 The real corpus is checked (R4.5)
  - Verified: the shipped `TrimBars` over all 638 files on the author's machine — **8 trimmed (1.3%)**, all genuine: the four pillarboxed DuckDuckGo images and four letterboxed Wallhaven ones. The naive version found 70
- [x] AC9 A Cloudflare page does not reach the log, and each named status is named (R3.1, R3.2)
  - Verified: `TestAPIErrorSaysWhatTheStatusMeans`, `TestSnippetIsOneShortLine`
- [x] AC10 Only an outage stops the run (R3.3, R3.4)
  - Verified: `TestOnlyAnOutageStopsTheRun`; the existing `TestWallhavenRunContinuesWithTheNextQueryAfterANon200Search` still pins that a 429 does not
- [x] AC11 `make test` passes
  - Verified: all packages ok
- [x] AC12 `make lint` is clean
  - Verified: 0 issues
- [x] AC13 `govulncheck ./...` is clean
  - Verified: no vulnerabilities found

## Risks & Assumptions

- **Rollback**: revert the commit; v4.2.6 is installable from the AUR and the
  GitHub artifacts. Nothing already downloaded is affected either way.
- **A false positive crops a good wallpaper.** This is the risk that shaped the
  design. Four guardrails, each individually able to refuse, and the detector
  checked against 638 real images rather than fixtures alone. The residual case
  is an image that is genuinely padded *and* whose content begins with a flat,
  extreme, hard-edged line — which is padding by any reasonable reading.
- **Wallhaven now decodes every download.** About a tenth of a second per 4K
  image, against a run that downloads ten. The re-encode, which is the
  expensive part, happens only for what is padded.
- **A trimmed Wallhaven JPEG is re-encoded at q95** and so is no longer the
  source's bytes. It is the only way to trim a file this plugin otherwise
  copies verbatim, and it happens to roughly one download in seventy.
- **The 429 message changed**, which an existing test pinned. Updated with its
  intent intact: a rate limit still does not stop the run.

## Gaps found

- **DuckDuckGo's `Failed to process image` says nothing useful.** During the
  same run, three downloads from `img.uhdpaper.com` failed with `decode image:
  image: unknown format`. Checked: that host answers `301 → 302 →` a 20 KB
  `text/html` page, so the decoder is right and the input is genuinely HTML.
  Checking `Content-Type` before decoding would turn three mystery lines into
  three explanations, and skip the decode. Deliberately out of scope; the user
  judged it not important.
- **A run that works looks like one that does not.** "Found 60 potential
  images" followed by one download line reads like a failure, when in fact 58
  were deduplicated against history. The skip reason only appears as a
  transient progress message. A per-term summary would fix it.
- Carried and still open: the Service section rebuilding on a moving timestamp
  (015), and nothing telling the user their unit is stale (015).

## Alternatives Considered

- **Trim existing files too.** Rejected: a false positive on a file already on
  disk is unrecoverable, and the rate does not justify it.
- **Re-encode every Wallhaven download so trimming is uniform.** Rejected:
  preserving the source's bytes is a feature of that plugin, and re-encoding
  638 images to fix four is the wrong trade.
- **Infer the bar colour rather than testing for extremes.** Rejected: padding
  is white or black in every observed case, and a detector that accepts any
  uniform colour is one that accepts a clear sky.
