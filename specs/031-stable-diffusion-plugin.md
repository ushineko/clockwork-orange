# Spec 031: Stable Diffusion plugin

**Issue**: #60

## Status: INCOMPLETE

- **Priority**: Medium
- **Estimated Complexity**: High
- **Branch**: `feat/sd-plugin`
- **Builds on**: spec 030 (spike; decided on ardanlabs/malina, build the
  native library in CI, keep the glibc floor)

## Executive Summary

<!-- Populate last. -->

## Context

v4 preserves the `stable_diffusion` config block but has no plugin behind it
(spec 010, D6). Spec 030 showed that `github.com/ardanlabs/malina` drives
stable-diffusion.cpp from Go with `CGO_ENABLED=0`, generates on CPU and Vulkan,
and can embed its native library. This spec turns that into a `stable_diffusion`
wallpaper source, a model manager, and the CLI and GUI surfaces for both.

The v2.9.5 plugin (`plugins/stable_diffusion.py`, PyTorch/diffusers) is the
behavioural reference for the config keys and the run flow: pick an enabled
prompt, generate `limit` images into `download_dir`, optionally Lanczos-resize
to 2560x1440, name them `sd_<timestamp>_<i>.png`, stamp `.last_run`, apply
retention.

## Requirements

- **R1 — Plugin.** `stable_diffusion` joins `plugins.Registry` after
  `duckduckgo_images`. Schema keys keep their v2.9.5 names, types, defaults and
  order so an existing block loads unchanged: `prompt` (string_list of
  `{term, enabled}`), `negative_prompt`, `model_id`, `steps` (30), `width`
  (768), `height` (512), `upscale` (true), `safety_checker` (true),
  `guidance_scale` (7), `download_dir`
  (`~/Pictures/Wallpapers/StableDiffusion`), `interval` (Daily), `limit` (1),
  `max_files` (50). New keys, appended: `seed` (0 = random), `threads` (0 =
  half the logical CPUs), `backend` (`auto`, `cpu`, `vulkan`), `device` (GPU
  index, -1 = library default). Unknown keys in the block are preserved (D6).
- **R2 — Run flow.** Same order as the other download plugins: ensure the
  directory, handle `process_blacklist`, honour the interval unless `force`,
  handle `reset`, generate, stamp `.last_run`, apply retention on `sd_*.png`,
  succeed when something was generated or the directory is non-empty.
  Dimensions are rounded down to a multiple of 8. One prompt is chosen per
  image from the enabled terms. Each saved image is recorded in history with
  source `stable_diffusion` (so retention only removes files the app made),
  checked against the blacklist, and reported through `Events.ImageSaved`.
  Progress is reported per image. Cancelling the context stops the run between
  images; a single image in flight cannot be interrupted.
- **R3 — `model_id` resolution.** A catalog name (`sd-1.5`) selects a pulled
  model. The v2.9.5 defaults `runwayml/stable-diffusion-v1-5` and the other
  Hugging Face ids it suggested map to a catalog entry when one exists. An
  existing file path is used as-is. Anything else fails with a message naming
  `models pull`. The default `model_id` becomes `sd-1.5`.
- **R4 — Safety checker.** stable-diffusion.cpp has no NSFW classifier. The key
  is kept for compatibility; when true the run logs once that it is not
  enforced. The schema description says so.
- **R5 — Native library loading.** `internal/sdlib` resolves a directory holding
  the stable-diffusion.cpp libraries: `CLOCKWORK_SD_LIB` if set (development
  and tests), otherwise the library embedded in the binary. The embedded copy
  lives behind a build tag so default builds and CI stay as they are; without
  the tag, and without the variable, the plugin fails with an error that says
  this build has no Stable Diffusion library. Extraction goes to
  `os.UserCacheDir()/clockwork-orange/sd/<content-hash>/` once, atomically, with
  a completion marker. Loading uses purego; `CGO_ENABLED=0` is kept for the
  daemon.
- **R6 — Model manager.** `internal/models` lists a curated catalog (name, size,
  licence, gated flag, pulled state), pulls a model with progress through
  `events.Events` and cancellation through `context.Context`, resumes partial
  files with HTTP Range, and verifies the pinned SHA-256 before the file is
  renamed into place. It does not use malina's `pkg/download`: that package
  grew the CLI from 20.5 MB to 74 MB (go-getter and its cloud SDKs), while
  `pkg/sd` adds 53 KB. Models live under `~/.local/share/clockwork-orange/models`
  on Linux, the platform equivalent elsewhere. The catalog is the single-file
  image models from malina's list (`sd-1.5`, `sdxl-base-1.0`) with the SHA-256
  Hugging Face publishes for each file.
- **R7 — Core operations and parity.** `core.ListModels` and `core.PullModel`
  take request structs and return result structs; the plugin is reached through
  the existing `RunPlugin`. CLI: `models list`, `models pull <name>`. GUI: a
  Models area in the Stable Diffusion plugin's settings with the same list,
  a Pull action, and progress that floats over the content (no reflow). The
  parity table gains both operations with no new allow-list entries.
- **R8 — Backend and device.** `backend`/`device` set the environment the
  native library reads (`GGML_VK_VISIBLE_DEVICES`) before the library loads in
  the process, so the first load wins; the plugin logs which it applied. The
  default Vulkan device on a machine with an integrated and a discrete GPU was
  the integrated one in spec 030, so the GUI offers `device`.
- **R9 — CI library build.** A CI job builds stable-diffusion.cpp at malina's
  pinned version on the existing Linux runner (CPU and Vulkan variants), fails
  if the library needs glibc above 2.39 (`objdump -T`), checksums the output,
  and feeds it to the build tag in R5. The pinned version moves with the
  `malina` module version. Windows and macOS libraries follow the same shape in
  a later spec.
- **R10 — Module and toolchain.** malina declares `go 1.27.0`; `go.mod` moves
  from `go 1.26.0`, and the lint pin (`LINT_VERSION`, `LINT_GO_TOOLCHAIN`, and
  the `config/.golangci-<version>.yml` file) moves with it because the pinned
  linter v2.12.2 was built with Go 1.26 and refuses a Go 1.27 module. The CLI
  size and `govulncheck` result are recorded in "Findings".

## Acceptance Criteria

- [x] `plugins list` shows `stable_diffusion` with the R1 schema in v2.9.5 key
      order, and a v2.9.5 `stable_diffusion` block loads and saves unchanged
      (unit test over a fixture; unknown keys preserved) (R1)
- [x] With a fake generator, `Run` honours the interval, `force`, `reset`,
      `limit`, retention, history recording and blacklist removal, and rounds
      dimensions to multiples of 8 (unit tests) (R2)
- [x] `model_id` resolution covers a catalog name, a v2.9.5 Hugging Face id, an
      existing path, and an unknown value with the `models pull` message (R3)
- [x] `safety_checker: true` logs the not-enforced notice once per run (R4)
- [x] `sdlib` resolves `CLOCKWORK_SD_LIB`, extracts an embedded library
      atomically and idempotently, and reports a clear error in a build without
      one (tests with a temp directory and a fake embedded filesystem) (R5)
- [ ] `models list` prints name, size, licence and pulled state; `models pull`
      downloads a real small bundle, resumes an interrupted pull, and a file
      with a wrong hash is rejected and removed (live test against Hugging Face,
      env-gated `CLOCKWORK_LIVE_NET=1`) (R6)
- [ ] The parity test passes with `core.ListModels` and `core.PullModel` and no
      new allow-list entries; the GUI Models area lists, pulls with cancel, and
      shows progress as a popup (R7)
- [ ] A binary built with the embed tag, `CGO_ENABLED=0` for the CLI, runs
      `clockwork-orange plugin run stable_diffusion --force` on this machine
      and writes a PNG from the real library and a real model; the GUI build
      does the same from the Plugins section (env-gated
      `CLOCKWORK_LIVE_SD=1` test for the CLI path) (R5, R7, R8)
- [x] `backend: vulkan` with `device: 0` generates on the RTX 4090 and the log
      names the applied device (R8)
- [ ] A CI job produces the library at the pinned version, enforces the glibc
      ceiling, and the packaged CLI and GUI pass `--self-test` (R9)
- [x] `go.mod` and the linter pin agree on the new Go version, `make test` and
      `make lint` pass, and `govulncheck` is clean or its findings are listed
      here (R10)

The live criteria (real library, real model, real download) are the
integration-boundary coverage for this spec; unit tests with a fake generator
do not satisfy them.

## Risks & Assumptions

- **Dependency weight.** malina's `pkg/download` brings go-getter and its cloud
  SDKs. If the CLI size or `govulncheck` result is unacceptable, replace the
  download wrapper with the spike's small downloader (`experiments/sd-spike/
  modelget`), which has no dependencies beyond the standard library, and keep
  malina only for `pkg/sd`.
- **Go version.** Moving to Go 1.27 may need a newer golangci-lint than the
  current pin; CI uses `go-version-file` with `check-latest`.
- **Library version lockstep.** A mismatch between the malina module and the
  native library crashes the process (spec 030). The loader must record the
  pinned version next to the library and refuse a mismatch before loading.
- **Binary size.** An embedded Vulkan library adds about 100 MB per binary;
  default builds carry none.
- **GPU selection and first run.** The default Vulkan device may be wrong; the
  first Vulkan run was about ten times slower than later ones (cause not
  confirmed).
- **Cancellation.** One image cannot be interrupted once started.
- **Memory.** A model stays resident while an image is generated; the daemon
  frees the context after each run. Resident size of the daemon during a run is
  recorded in the E2E plan.
- **Models and licences.** Models are 2 to 7 GB; the catalog shows the licence
  before a pull. Gated models need `HF_TOKEN` and are out of scope for the first
  catalog.
- **Rollback.** The plugin is additive; remove it from the registry. The
  config block already survives untouched.
- **Assumption.** The `stable_diffusion` block key set above is the only
  compatibility surface; nothing else read it.

## E2E Test Plan

Environment: this desktop (CachyOS, KDE Plasma 6, RTX 4090 plus Intel iGPU),
build with the embed tag and the spec 030 library.

1. `make build` with the embed tag; run `clockwork-orange models pull sd-1.5`
   (AC: models) and `models list` (pulled state shown).
2. `clockwork-orange plugin run stable_diffusion --force
   --plugin-config backend=vulkan --plugin-config device=0` writes
   `sd_*.png` into the download directory and the history shows the entry
   (AC: live generation, backend).
3. Repeat with `backend=cpu` and compare timing and the log lines.
4. Build the GUI, open Plugins > stable_diffusion, edit a prompt, Pull a model
   with cancel, and Run (AC: GUI).
5. Enable the plugin as the wallpaper source, run the daemon for two intervals,
   record resident memory during a run, and confirm retention.
6. Package check: `--self-test` on the produced CLI and GUI.

Results are recorded here after the run.

### Prod visibility

Not applicable: desktop software with no server side; nothing is released
from this branch until the CI library job (R9) is green. Operator agreement to
be confirmed at review.

## Alternatives Considered

- Considered Pendra-AI/stable-diffusion-go as a second backend; rejected for
  now. Spec 030 keeps it on record as a later fallback only.
- Considered malina's prebuilt Linux libraries; rejected because they need
  glibc 2.43 against the 2.38 the GUI needs today.
- Considered shelling out to `sd-cli` or a local SD server; rejected in spec 030
  in favour of an in-process library.

## Technical Notes

- Reference experiments: `experiments/sd-spike/malina` (load, generate),
  `experiments/sd-spike/malina-embed` (embed and extract),
  `experiments/sd-spike/modelget` (resume and checksum).
- malina's `sd.Load(dir)` and `sd.Init(dir)` take the library directory;
  libraries find each other through `RUNPATH=$ORIGIN`, so a flat directory
  works.
- `GGML_VK_VISIBLE_DEVICES=<index>` selects the Vulkan device.
- Set `NThreads` explicitly: the default was half as fast in spec 030.
- Known catalog hash source: Hugging Face LFS `oid` per file.

## Findings

Run on 2026-11-12 on this desktop (CachyOS, KDE Plasma 6, RTX 4090 plus Intel
iGPU) in an isolated `HOME`, with the Vulkan library from spec 030 staged by
`make sd-lib` and embedded by `make build-sd` / `make build-gui-sd`.

**Working end to end**

- `models list` shows both models as not pulled; `models pull sd-1.5` downloaded
  4.3 GB, verified the pinned SHA-256 and renamed it into
  `~/.local/share/clockwork-orange/models/sd-1.5/`; `models list` then shows it
  pulled.
- `clockwork-orange plugin run stable_diffusion --force` with `backend=vulkan`,
  `device=0`, 25 steps wrote a 2560x1440 PNG (3.7 MB) and recorded it in
  history. The prompt "a lighthouse on a cliff at sunset, dramatic clouds,
  photorealistic, 4k" produced a lighthouse at sunset. 17 s on the first run
  including shader compilation, 2.9 s for generation on a warm run.
- The embedded library extracted once to `~/.cache/clockwork-orange/sd/<hash>/`
  and loaded from there with `CGO_ENABLED=0`; `--self-test` passes and lists
  four plugins.
- `TestLiveGenerate` (gated by `CLOCKWORK_LIVE_SD=1`) passes against the real
  library and checkpoint in 14 s.
- `go test -race -tags parity ./...` passes, including the parity guard with
  `models list` and `models pull` and no new allow-list entries.
- `govulncheck ./...`: no vulnerabilities found.

**Size and linking**

- The CLI is 20.5 MB with malina's `pkg/sd` imported (53 KB more than before)
  and 74 MB with `pkg/download` imported, which is why R6 uses its own
  downloader.
- With the Vulkan library embedded the CLI is 121 MB and the GUI 140 MB.
- Any binary that imports malina is dynamically linked against glibc
  (`libdl`, `libpthread`, `libc`; purego with `CGO_ENABLED=0`), where the CLI
  was statically linked before. It has no versioned glibc symbol requirement,
  but it no longer runs on a system without glibc (musl, Alpine). README text
  that calls the CLI static is now wrong for Linux builds from this branch.

**Deviations from v2.9.5**

| # | v2.9.5 | Here | Why |
|---|---|---|---|
| SD1 | `upscale` stretched 768x512 to 2560x1440 (Lanczos) | `imaging.CoverResizeCrop`: scale to cover, centre-crop | The stretch distorted a 3:2 image into 16:9. |
| SD2 | A run that made nothing still succeeded when the directory had any entry | Succeeds only when it generated an image or `sd_*.png` files already exist | `.last_run` is always written, so "non-empty" was always true. |
| SD3 | `safety_checker` ran the diffusers filter | Accepted, not enforced, logged once | stable-diffusion.cpp has no classifier. |
| SD4 | Images were not recorded in history | Recorded under `generated://stable_diffusion/<name>` | Retention only removes files the app made (spec 023). |
| SD5 | Hugging Face model ids | Mapped to catalog names where one exists; others need a path | No diffusers model cache. |

A plain-string `prompt` is a comma-separated list of prompts, as in v2.9.5;
`--plugin-config '{"prompt":"a, b"}'` picks one of `a` or `b`. The first run in
this session used it that way and drew houses. A single prompt that contains
commas is passed as a one-item list.

**Not done**

- R9, the CI library build, and the glibc ceiling check. The libraries used here
  are malina's upstream build, which needs glibc 2.43; the floor decision in
  spec 030 depends on this job.
- R10 is done: `go.mod` is on Go 1.27.0, the linter moved to v2.15.0 (built with
  Go 1.27.0; `LINT_GO_TOOLCHAIN` and `config/.golangci-v2.15.0.yml` moved with
  it), `make lint` reports 0 issues, `make test` passes and `govulncheck` is
  clean. The README, CONTRIBUTING and the Makefile no longer call the Linux CLI
  static.
- No live test of `models pull` against Hugging Face: the catalog files are
  4.3 GB and larger. Resume, checksum failure, cancellation and the gated
  case are covered by unit tests against a local HTTP server, and a real pull
  was run by hand above.
- The Stable Diffusion section was rendered on 2026-11-12 (KDE Plasma 6, Wayland,
  fynedesygn 0.1.114 on Fyne 2.8.3-pre) in Breeze Dark and Breeze Light with the
  library's `tools/screenshot.sh`, under a throwaway `HOME` and
  `CLOCKWORK_LOCK_DIR`: the sparkle icon in the navigation, the Models card with
  both models, size, licence and a Pull button each, and the generated form
  below it. The first capture showed the Pull buttons stretched to the height of
  the two text lines beside them and the model field's caption carrying a CLI
  hint in backticks; both were fixed and re-captured. A pull in flight and the
  Review tab were not captured, and no other section was re-rendered on the new
  Fyne. The pull popup carries the download's real fraction through
  `Shell.BusyProgress` (fynedesygn 0.1.114, spec 078); the library's own
  rendered reading of that popup is in its spec 078.
- Cancelling a run stops it between images; an image in flight finishes.
- Windows and macOS libraries and the packaged-artifact `--self-test`.

### Gaps found

- `fynedesygn`: the busy popup had a fixed caption and no determinate progress
  value, so a multi-gigabyte download showed only a spinner. Closed:
  `Shell.BusyProgress(fraction, caption)` shipped in 0.1.114 (fynedesygn #230,
  spec 078) and the model pull uses it. The plugin run keeps the run dialog's
  progress bar, which already shows a percentage.
