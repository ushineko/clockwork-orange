# Spec 030: Spike — a native-Go Stable Diffusion stack

**Issue**: #57

## Status: INCOMPLETE

- **Priority**: Low
- **Estimated Complexity**: Medium (spike; throwaway code)
- **Branch**: `docs/spec030-sd-spike` (spec); experiments live outside `internal/`

## Executive Summary

<!-- Populate last. -->

## Context

The `stable_diffusion` config block is preserved on save (on-disk
compatibility with 2.9.x) but v4 has no plugin behind it. A research pass
(2026-11-12) found no Go reimplementation of Stable Diffusion inference. Every
Go option wraps the C++ library
[stable-diffusion.cpp](https://github.com/leejet/stable-diffusion.cpp), and the
cgo-free ones load its shared library at runtime through `purego`.

Candidates found:

| Candidate | Approach | Known concern |
|---|---|---|
| `github.com/l8bloom/gosd` | purego | New (May 2026); performance claims unverified |
| `github.com/Pendra-AI/stable-diffusion-go` | purego, prebuilt libs per release | v0.4.0 notes: `GenerateImage` returns nil/false on SysV amd64 (Linux) |
| `github.com/seasonjs/stable-diffusion` | purego, bundled libs | v0.0.4-beta1; no CUDA/ROCm on Linux |
| `github.com/ardanlabs/malina` | upstream release artifacts, pinned version | Not yet examined |

Excluded from the first round: `dennwc/gosd` (cgo, Linux x64 only) and
`hjkoskel/bindstablediff` (CPU only).

The `CGO_ENABLED=0` rule for the daemon and the single-binary packaging
constraint both survive if the shared library is embedded (`go:embed`, one
library per OS behind build tags) and extracted to the user cache directory
under a content-hash name before `dlopen`/`LoadLibrary`. That is accepted as an
approach; the spike measures what it costs.

This spec decides which candidate, if any, is worth a plugin spec. It does not
build the plugin.

## Requirements

- **R1 — Standalone experiments.** Each candidate is exercised in its own
  throwaway module under `experiments/sd-spike/<candidate>/` (or a scratch
  directory outside the repo), with no import of `internal/*` and no change to
  `go.mod` of the main module. Nothing from the spike ships.
- **R2 — Build and dependency complexity.** For each candidate, record on
  Linux (this machine) from a clean checkout: the commands needed to build and
  run, whether `CGO_ENABLED=0` builds, the native library source (prebuilt
  release vs built from source), the pinned stable-diffusion.cpp version,
  extra system packages, shared-library size, and wall-clock for a cold build.
  Fewer steps and fewer system dependencies score better.
- **R3 — Real generation.** Each candidate that builds generates at least one
  image from a real model on CPU, and on a GPU backend where the machine
  supports one (Vulkan first; CUDA/ROCm if present). One small SD 1.x/SDXL-turbo
  class model is enough. The image is saved as PNG with the prompt, seed,
  steps, model, backend, and wall-clock recorded next to it. A candidate that
  returns no image, crashes, or needs patching to run is recorded as a
  failure with the evidence, not worked around silently.
- **R4 — Embedded-library load path.** For the best one or two candidates,
  show that the shared library can be embedded with `go:embed`, extracted to a
  cache directory, and loaded from that custom path with `CGO_ENABLED=0`. If a
  binding cannot load from a caller-supplied path, record that.
- **R5 — Model acquisition frontend.** Demonstrate a user-friendly way to get
  models, as a standalone prototype (CLI is enough; a Fyne mock is optional):
  list a small curated set of models, show size and licence, download with
  progress and resume, verify a checksum, and store under the user data
  directory. Sources are Hugging Face file URLs; no account-bound or gated
  model is required for the demo. Record what a GUI would need from the
  prototype's API (progress events, cancellation via `context.Context`).
- **R6 — Findings write-up.** The spec's "Findings" section records, per
  candidate, the R2–R4 results, a recommendation (adopt / hold / reject), and
  the follow-up spec outline (plugin shape, CLI/GUI parity operations, config
  fields). Anything the spike shows `fynedesygn` lacks for a model-download UI
  goes under "Gaps found".

## Acceptance Criteria

- [ ] Each of gosd, Pendra-AI/stable-diffusion-go and seasonjs/stable-diffusion has an
      experiment that builds from a clean checkout; malina is built or recorded
      as not buildable with the reason (R1, R2)
- [ ] A build/dependency table in "Findings" lists steps, `CGO_ENABLED=0`
      result, native-library source, pinned stable-diffusion.cpp version,
      system packages, library size and cold-build time for every candidate (R2)
- [ ] At least one candidate generates a PNG from a real model on CPU through
      the actual shared library, not a mock; prompt, seed, steps, model, backend
      and timing are recorded with the image (R3)
- [ ] GPU generation is attempted where the hardware allows, and the result
      (image or failure with output) is recorded (R3)
- [ ] Every candidate that fails to generate has the failure evidence
      (command, output, version) recorded (R3)
- [ ] For the leading candidate(s), a binary built with `CGO_ENABLED=0`
      embeds the shared library, extracts it to a cache directory, loads it
      from that path, and generates an image (R4)
- [ ] A model-download prototype lists curated models with size and licence,
      downloads with progress, resumes an interrupted download, and rejects a
      file whose checksum does not match (R5)
- [ ] The prototype's download function takes a `context.Context` and reports
      progress through a channel or callback that a GUI could render (R5)
- [ ] "Findings" holds an adopt / hold / reject recommendation per candidate and
      a follow-up spec outline, and "Gaps found" lists any `fynedesygn`
      shortfalls (R6)
- [ ] No spike code, model files, or generated images are committed to the main
      module; sample PNGs small enough to review may be committed under
      `specs/assets/030/` (R1)

The real-system criteria above (actual library load, actual generation, actual
network download) are the integration-boundary coverage for this spike; none
of them may be satisfied by a fake.

## Risks & Assumptions

- **Binding maturity.** gosd is new; Pendra has a documented Linux amd64
  limitation; seasonjs is beta. A candidate failing is an expected outcome.
- **Native library drift.** The stable-diffusion.cpp C API changes between
  releases; every candidate pins a version. The recommendation must name the
  version it was tested with.
- **GPU availability.** The Linux box's GPU backend may not match what a
  candidate ships. Windows and macOS are not covered; Metal and Windows
  Vulkan/DirectML behaviour stays unverified and goes into follow-up work.
- **Model licences.** Models carry different licences; the curated list records
  each licence and avoids gated models for the demo.
- **Disk and network.** Models are 2–7 GB; downloads go to the scratchpad or a
  throwaway directory, not the repository.
- **Supply chain.** Native libraries are downloaded binaries. Record the
  source URL and checksum for each; none is loaded without the checksum
  being recorded. Dependencies are added only in the experiment modules.
- **Tool installation.** Any missing tool (Vulkan SDK, cmake) is asked about
  before installing.
- **Rollback.** Nothing ships; delete `experiments/sd-spike/`.
- **Assumption.** A single curated model is enough to prove "models can be
  called"; model quality is out of scope.

## E2E Test Plan

Not applicable: spike with no deployed artifact. Verification is the
acceptance criteria above, run locally on Linux; results are recorded in
"Findings". Operator agreement to be confirmed at review.

### Prod visibility

Not applicable: nothing is released.

## Alternatives Considered

- Considered driving the `sd` CLI or a local SD server (AUTOMATIC1111, ComfyUI)
  as a subprocess or HTTP plugin; not rejected, but kept out of this spike
  because the question asked is whether an in-process Go stack is viable. It
  stays the fallback if no candidate passes.
- Considered a cgo binding (`dennwc/gosd`); rejected for the first round
  because it breaks `CGO_ENABLED=0` and is Linux x64 only.
- Considered ONNX Runtime from Go (`onnxruntime_go`) with exported SD models;
  not examined in the research pass, so deferred.

## Technical Notes

- `purego` needs no C toolchain; on Linux it loads via `dlopen`, on Windows via
  `LoadLibrary`, on macOS via the system loader.
- Library embedding sketch: `//go:embed lib/linux_amd64/libstable-diffusion.so`
  behind a `//go:build linux && amd64` tag; extract to
  `os.UserCacheDir()/clockwork-orange/sd/<sha256>/` if absent.
- macOS would need the extracted dylib signed and notarized with the app
  (issue #36); Windows extraction may draw antivirus attention. Both are
  follow-up concerns, not spike scope.
- Reference: [stable-diffusion.cpp](https://github.com/leejet/stable-diffusion.cpp),
  [gosd](https://github.com/l8bloom/gosd),
  [Pendra-AI/stable-diffusion-go](https://github.com/Pendra-AI/stable-diffusion-go),
  [seasonjs/stable-diffusion](https://github.com/seasonjs/stable-diffusion),
  [ardanlabs/malina](https://pkg.go.dev/github.com/ardanlabs/malina).

## Findings

Pending.

### Gaps found

Pending.
