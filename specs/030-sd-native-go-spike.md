# Spec 030: Spike — a native-Go Stable Diffusion stack

**Issue**: #57

## Status: COMPLETE

- **Priority**: Low
- **Estimated Complexity**: Medium (spike; throwaway code)
- **Branch**: `docs/spec030-findings` (findings); experiments live in
  `experiments/sd-spike/`, each its own Go module outside the main module

## Executive Summary

Four Go bindings for stable-diffusion.cpp were run on Linux with SD 1.5.
Pendra-AI/stable-diffusion-go and ardanlabs/malina both generate images on CPU
and on the RTX 4090 over Vulkan, both embed and load their library with
`CGO_ENABLED=0`, and both build in seconds. gosd works on CPU only, and only
against its exact pinned library; seasonjs crashes on every library tried.
Decision (operator): standardize on malina, with no fallback implemented. It
ships a model catalog, pinned and verified library installs, and resumable
downloads. Its prebuilt Linux libraries need glibc 2.43, above the 2.38 the GUI needs
today. Decision (operator): keep the current floor and build
stable-diffusion.cpp in CI. Pendra is recorded as a possible later fallback only. Reviewers
should read "Findings" first, then "Follow-up spec outline".

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

- [x] Each of gosd, Pendra-AI/stable-diffusion-go and seasonjs/stable-diffusion has an
      experiment that builds from a clean checkout; malina is built or recorded
      as not buildable with the reason (R1, R2)
- [x] A build/dependency table in "Findings" lists steps, `CGO_ENABLED=0`
      result, native-library source, pinned stable-diffusion.cpp version,
      system packages, library size and cold-build time for every candidate (R2)
- [x] At least one candidate generates a PNG from a real model on CPU through
      the actual shared library, not a mock; prompt, seed, steps, model, backend
      and timing are recorded with the image (R3)
- [x] GPU generation is attempted where the hardware allows, and the result
      (image or failure with output) is recorded (R3)
- [x] Every candidate that fails to generate has the failure evidence
      (command, output, version) recorded (R3)
- [x] For the leading candidate(s), a binary built with `CGO_ENABLED=0`
      embeds the shared library, extracts it to a cache directory, loads it
      from that path, and generates an image (R4)
- [x] A model-download prototype lists curated models with size and licence,
      downloads with progress, resumes an interrupted download, and rejects a
      file whose checksum does not match (R5)
- [x] The prototype's download function takes a `context.Context` and reports
      progress through a channel or callback that a GUI could render (R5)
- [x] "Findings" holds an adopt / hold / reject recommendation per candidate and
      a follow-up spec outline, and "Gaps found" lists any `fynedesygn`
      shortfalls (R6)
- [x] No spike code, model files, or generated images are committed to the main
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

Run on 2026-11-12: CachyOS (glibc 2.44), Intel i9-14900K, NVIDIA RTX 4090 plus
an Intel iGPU, Go 1.27.3. Every generation used the same inputs: model
`stable-diffusion-v1-5-pruned-emaonly-Q8_0.gguf` (second-state, SHA-256
`d0555243…bf94`, verified), prompt "an orange cat on a beach, photo", seed 42,
512x512, 20 steps, CFG 7. Experiment sources are in `experiments/sd-spike/`;
the `*-embed` programs expect the library staged in a `lib/` directory next to
`main.go`. Sample images are in `specs/assets/030/`.

### Build and dependency complexity

| | Pendra-AI/stable-diffusion-go v0.4.0 | ardanlabs/malina v1.2.2 | l8bloom/gosd | seasonjs/stable-diffusion |
|---|---|---|---|---|
| Steps to a running binary | `go build`; download one library archive, check `checksums.txt` | `go build`; `malina install` (verified, pinned) | `go build`; download the matching upstream release | `go build`; library must be found by the caller |
| `CGO_ENABLED=0` build | yes | yes | yes | yes |
| Cold build | 1.5 s | 12 s (malina CLI); under 1 s (experiment, warm module cache) | 4 s (including `go get`) | not timed |
| Library source | own prebuilt per tag, built from a pinned commit plus carried patches | upstream release, pinned and digest-verified | upstream release, must match `stable_diffusion.release` | none documented; the repo's Linux library is a Git LFS pointer (132 bytes) |
| Pinned stable-diffusion.cpp | master-1031-c6dfa9d | master-1088-32d516c | master-679-f3fd359 (HEAD), master-656-0e4ee04 (v0.11.0) | none |
| Library layout | one self-contained file | about 30 files (ggml split, 15 CPU variants, webp) | same as malina | n/a |
| Library size (Linux) | CPU 42 MB, Vulkan 83 MB, CUDA 349 MB | CPU 67 MB, Vulkan 107 MB (the ggml and webp libraries appear as separate unversioned and versioned copies) | not measured | n/a |
| glibc needed by the library | 2.34 | 2.43 | 2.38 | n/a |
| Extra system packages | libstdc++, a Vulkan loader for Vulkan, CUDA 13 runtime for CUDA | libstdc++, a Vulkan loader for Vulkan | same | n/a |
| Binary links glibc dynamically | yes (purego `libdl`) | yes | yes | yes |

The non-embedded experiment binaries are 3.5 MB each. No candidate needed a C
toolchain, cmake, or the Vulkan SDK. The `CGO_ENABLED=0` binaries are
dynamically linked against glibc, so they do not run on musl.

### Generation results

| Candidate | Library | Backend | Result | Time (20 steps) |
|---|---|---|---|---|
| Pendra | own v0.4.0 | CPU, 16 threads | image (`pendra-cpu.png`) | 1 m 31 s |
| Pendra | own v0.4.0 | Vulkan, default device | image; speed matches the Intel iGPU run with `GGML_VK_VISIBLE_DEVICES=1` (2 m 31 s) | 2 m 32 s |
| Pendra | own v0.4.0 | Vulkan, `GGML_VK_VISIBLE_DEVICES=0` (RTX 4090) | image (`pendra-vulkan-rtx4090.png`) | 20.0 s first run, 1.8 s warm |
| Pendra | own v0.4.0 | CUDA | **fails at load**: `libcublasLt.so.13: cannot open shared object file` (no CUDA 13 runtime installed; not installed, per the tool-installation rule) | n/a |
| malina | upstream master-1088 | CPU, default threads | image | 3 m 05 s |
| malina | upstream master-1088 | CPU, `NThreads=16` | image | 1 m 31 s |
| malina | upstream master-1088 | Vulkan, RTX 4090 | image from the Q8 model | 19.6 s first run, 1.8 s warm |
| malina | upstream master-1088 | Vulkan, RTX 4090 | image from malina's own catalog download, `sd-1.5` f32 safetensors (`malina-vulkan-rtx4090-sd15-f32.png`) | 5.8 s |
| gosd HEAD | upstream master-679 (pinned) | CPU, default threads | image | 1 m 58 s |
| gosd HEAD | upstream master-679 (pinned) Vulkan | Vulkan | **`SIGILL: illegal instruction`** inside the native library after load | n/a |
| gosd HEAD / v0.11.0 | any other upstream version | CPU or Vulkan | **segfault** in `ContextParamsInit` (struct layout mismatch) | n/a |
| seasonjs `main` (v2 package) | Pendra master-1031, upstream master-679, upstream master-1088 | CPU | **crash** in all three (`unexpected fault address` / `unknown caller pc`) | n/a |
| seasonjs v1 (root package) | bundled | n/a | not runnable: the bundled Linux library is a Git LFS pointer, absent from the module download | n/a |

Observations:

- **Default Vulkan device is wrong on this machine.** With an iGPU and a dGPU
  present the default picked the iGPU, which was slower than the CPU.
  `GGML_VK_VISIBLE_DEVICES` fixes it. A GUI needs a device picker; the daemon
  needs a config key.
- **The first Vulkan run is about ten times slower** (19.5 s against 1.8 s warm),
  consistent with shader compilation or cache warm-up; the cause was not
  confirmed. The UI should say so once.
- **Version lockstep is strict.** gosd and seasonjs crash rather than
  error when the library does not match the binding. Only Pendra (its build scripts include an
  ABI-layout check and an expected-symbol list; not exercised here) and malina (pinned, digest-verified
  install) guard against it.
- **CPU thread default matters.** malina's default was half as fast as
  `NThreads=16`; the plugin must set it.
- The CPU image was visually checked: a recognisable orange cat on a beach.

### Embedded library (R4)

| Candidate | Embedded | Binary | Extract (first / later run) | Result |
|---|---|---|---|---|
| Pendra, Vulkan | one file with `go:embed` | 87 MB | 62 ms / 34 ms | extracted to `~/.cache/<sha256-prefix>/`, loaded from that path with `CGO_ENABLED=0`, image generated on the RTX 4090 |
| malina, Vulkan | whole directory with `embed.FS`, flattened on extraction | 111 MB | 75 ms / under 1 ms (marker file) | same, image generated |

Both loaders take a caller-supplied directory (`Load(dir)`, `sd.Load(dir)`).
gosd reads `GOSD_DYN_LIB` and seasonjs reads `DIFFUSION_LIB`, which works with
`os.Setenv` before loading. malina's libraries find each other through
`RUNPATH=$ORIGIN`, so a flat directory works. `libstdc++.so.6` and
`libgcc_s.so.1` still come from the host.

### Model acquisition (R5)

`experiments/sd-spike/modelget` is a CLI prototype: curated catalog with size
and licence, `list`, `pull`, context cancellation, a progress channel, HTTP
Range resume, SHA-256 verification, and a `.part` file renamed only after the
checksum passes. Verified against Hugging Face: a fresh pull; a second pull of a
valid file is skipped; `-badsum` fails with `checksum mismatch` and removes the
partial; an interrupted 1.7 GB pull left a 90 MB `.part` and the next run
completed with the expected SHA-256; a truncated 3 MB `.part` resumed from 3 MB
and verified.

malina already ships the equivalent: `pkg/download` has `Catalog()`,
`GetBundleWithProgress(ctx, name, dest, tracker)`, Range resume, per-bundle
licence and gated flags, and a `manifest.json` mapping roles to paths. Pulling
`sd-1.5` (4.3 GB) and `realesrgan-x4-anime` (17 MB) worked and the pulled model
generated an image. It lacks per-file checksums for models (`BundleFile` has
`Role`, `Filename`, `URL`, `Size` as a display string) and its progress type is
go-getter's `ProgressTracker`, not a channel.

### Recommendations

| Candidate | Verdict | Reason |
|---|---|---|
| malina | **adopt** (decided: the single stack) | Works on CPU and Vulkan, pinned and verified library install, model catalog with resume, release 1.2.2 dated the day of the spike. Costs: needs Go 1.27 (module `go 1.27.0`, main module is `go 1.25.0`), its prebuilt Linux libraries need glibc 2.43, and `pkg/download` pulls in go-getter. |
| Pendra | **hold**: secondary option for a possible later fallback; not to be implemented now | Works on CPU and Vulkan; one self-contained library, glibc 2.34, ABI checks, checksummed releases. No model catalog, CUDA needs a CUDA 13 runtime, and its pinned stable-diffusion.cpp build is 57 builds behind malina's. |
| gosd | **reject** | Vulkan release SIGILLs on the pinned version; every other version segfaults; CPU only worked in one combination. |
| seasonjs | **reject** | Crashes on every library tried; no pinned library; Linux library not shipped. |

**glibc floor (checked, not settled).** malina's upstream libraries need glibc
2.43 (Ubuntu 26.04), which excludes Debian 12/13 and Ubuntu 22.04–24.04. The
operator expected the project's binaries to already require that. Checked on
2026-11-12 against the local builds in `bin/`: the GUI needs glibc 2.38
(`objdump -T`), and the CLI/daemon is statically linked with no glibc
requirement. CI builds on `ubuntu-latest` (24.04, glibc 2.39). The floor is
therefore raised by adopting malina's prebuilt libraries: GUI 2.38 to 2.43, and
the daemon from none to 2.43 once it loads a shared library. Decision (operator, 2026-11-12): keep the floor. The plugin spec builds
stable-diffusion.cpp in CI at malina's pinned version, on the same runner as
the other Linux builds, and embeds that library instead of malina's prebuilt
one. Pendra (glibc 2.34)
stays on record as a later fallback; nothing for it is built.

### Follow-up spec outline

1. **Core operation** `GenerateImage` in `internal/core`: request struct (prompt,
   negative prompt, size, steps, seed, model, backend, device), result struct
   (path, seed, timings); CLI and GUI render only (parity allow-list entry
   until both exist).
2. **Native loader** in `internal/platform`: embedded library per OS behind
   build tags, extract to the cache dir by content hash, `Load(dir)`; a fake
   for tests. Daemon keeps `CGO_ENABLED=0`.
3. **Model manager**: catalog (name, size, licence, gated, SHA-256), download
   with resume and checksum, progress through `internal/events`, stored under
   the user data directory; either wrap `malina/pkg/download` and add
   checksums, or adopt `modelget`.
4. **Config**: fill the existing `stable_diffusion` block (model, backend,
   device index, threads, steps) without disturbing preserved unknown keys.
5. **GUI**: model list with size and licence, download progress popup (not
   inline, per the no-reflow rule), device picker, first-run shader-compile
   notice.
6. **CI library build** (decided): build stable-diffusion.cpp at malina's
   pinned version on the existing Linux runner, with Vulkan and CPU variants,
   record its glibc requirement with `objdump -T` and fail the job above 2.39,
   checksum the output, and embed it. The version must move in lockstep with
   the `malina` module version. This adds cmake and a C++ toolchain to CI only;
   the Go build keeps `CGO_ENABLED=0`.
7. **Integration test**: an env-gated `CLOCKWORK_LIVE_SD=1` test that generates
   one image from a small model through the real library.

### Gaps found

- `fynedesygn`: no download/progress widget with cancel and resume state, and
  no list row that shows size, licence and a gated badge. The spike only built
  a CLI, so the exact gap is untested; the model manager GUI spec should check
  `docs/design-system.md` first.
- No library in the spike checks a model file's checksum; the project must add
  it for models.
