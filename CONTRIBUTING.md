<!-- Generated from the shared contributing policy. Edit the template and the project fragments, then re-render; do not hand-edit this file. -->

# Contributing to Clockwork Orange

Thanks for your interest. This is a personal project, and external pull requests
are welcome. This guide states the policy a PR is held to, and what the
maintainers will do with it.

## TL;DR

- Only elected maintainers merge, and a contributor PR needs a maintainer's
  approving review. See [MAINTAINERS.md](MAINTAINERS.md).
- AI-written and hand-written PRs are both accepted, and both get the same
  review. Either may be rejected if it does not meet the standards below.
- Every PR needs tests **and**, for anything touching hardware or device
  support, an end-to-end reading from a real system — pasted into the PR.
  A PR without that collateral may be rejected.
- Keep the diff scoped: one logical change, no drive-by reformatting.
- No `Co-Authored-By` trailers and no AI-attribution footers in commits or the
  PR description.

## Who merges

Merge rights belong to the maintainers listed in
[MAINTAINERS.md](MAINTAINERS.md), and to nobody else. A PR from anyone who is
not a maintainer — human or agent — needs an approving review from a maintainer
before it lands, and the maintainer does the merging.

Maintainers merge their own work. These are small projects with a short
maintainer list, and a rule that forced a second maintainer's sign-off on every
change would simply stop work. The review requirement is there to gate
contributions coming in, not to make maintainers wait on each other.

Maintainers are elected, not self-appointed. The process, the current roster,
and how to be considered are all in [MAINTAINERS.md](MAINTAINERS.md).

A maintainer may merge a PR as-is, modify it before merging, hold it pending
changes, or close it. Closing is not a judgement of the contributor; it most
often means the change does not fit the project's direction, and that is the
maintainer's call to make.

## AI-written contributions

Code written with an AI assistant is fine. It is reviewed exactly like
hand-written code, held to the same standards, and modified by maintainers where
needed.

One rule makes that workable: **you must understand what you submitted.** If you
cannot explain what the change does, how it behaves at the edges, and why it
fits the existing design, the PR will be closed. Review questions go to the
person who opened the PR, not back to a model.

Specifically, a PR is likely to be rejected if it:

- adds a plausible-looking abstraction the project did not ask for,
- restates existing behaviour in new words without changing it,
- carries generated commentary, diagnosis dumps, or implementation-plan prose
  in the diff or the PR body,
- reformats or "improves" code outside the change,
- or claims a test or an e2e reading that was not actually run.

Do not add `Co-Authored-By` trailers or "Generated with …" footers to commits or
to the PR description. A PR containing them will be asked to amend.

## Tests and e2e collateral

Two things are required, and they are separate requirements.

**1. Tests.** Any behaviour change adds or updates tests. Tests encode
behavioural contracts — what the code does — not internals. A test that breaks
on a pure refactor with no behaviour change is testing the wrong thing. Paste
the test run summary into the PR.

**2. An e2e reading from a real system.**

Clockwork Orange sets wallpaper and lock-screen images on three platforms
through three entirely different mechanisms. Every one of them can accept a
call and change nothing on screen. Any change to a platform backend, the
daemon, a download plugin, or the image databases needs a reading from a real
desktop session on each platform it affects.

Paste one reading per affected platform. At minimum:

- the platform and version (Plasma 6.x on Wayland or X11, Windows 10/11 build,
  macOS 13+);
- the command you ran and its real output;
- **confirmation the image actually changed on screen** — on Plasma in
  particular, a scripting call can report success and set nothing;
- for the lock screen: confirmation the lock screen itself changed, verified by
  locking, not inferred from the desktop;
- for the daemon: evidence of at least two rotations on the timer, not one
  manual trigger;
- for a download plugin: the source, how many images were fetched, and
  confirmation the de-duplication and blacklist were honoured on a second run;
- for a database or config change: confirmation a 2.9.x-era config and database
  are still read, since backwards compatibility is a promise here.

If you can only test one platform, say so and say which. A maintainer will take
the readings for the others or hold the PR.

Report what you actually observed. Describe the hardware, the OS and the
software versions involved, the command you ran, and its real output. "Works on
my machine" is not a reading. If a reading cannot be taken — no access to the
device, platform not available to you — say so plainly in the PR and say what
*was* verified; a maintainer will decide whether to take the reading or hold the
PR. Claiming a reading that was not taken is the one thing that will get a
contributor's future PRs declined on sight.

## Scope

A wallpaper and lock-screen manager for KDE Plasma 6, Windows 10/11 and
macOS 13+. It rotates the wallpaper (and on Plasma the lock-screen image) on a
timer from local folders and from download plugins, remembers every image it
has fetched so nothing is downloaded twice, and keeps a blacklist.

Version 4 is native Go: one static binary that is also the daemon, plus a Fyne
desktop window. No Python, no Qt, no interpreter.

In scope: platform backends, rotation policy, download plugins, the databases,
the daemon and the GUI. Config files and databases from the 2.9.x releases must
keep being read as-is — breaking that is a direction change. Out of scope:
reintroducing a Python or Qt runtime dependency, and image editing.

Changes that alter the project's direction — the interaction model, the
architecture, persistence formats, or the public interface — start as a GitHub
Discussion, not as a PR. A PR that changes direction without prior alignment
will likely be closed regardless of its quality.

## Development setup

```bash
git clone git@github.com:ushineko/clockwork-orange.git
cd clockwork-orange
make setup        # installs the pinned golangci-lint
make test         # no desktop session required
make lint
make build        # CLI / daemon
make build-gui    # desktop window (needs CGO, OpenGL and X11/Wayland headers)
```

Go 1.26 or newer. `make test` runs headless. Exercising a platform backend
needs a real desktop session on that platform.

## What gets checked on your PR

| Required from you | Not required from you |
| --- | --- |
| `make test` passes | Spec files in `specs/` |
| `make lint` is clean | Validation reports |
| Tests for the behaviour you changed | Version bump or release tag |
| An e2e reading per affected platform | `release_version.sh` runs |
| 2.9.x config and database compatibility preserved | |
| No secrets in code, logs, or pasted output | |
| No `Co-Authored-By` / AI-attribution trailers | |

The maintainer's own workflow (spec files under `specs/`, validation reports,
release tagging) is internal cadence. **External contributors are not expected
to write specs or validation reports, bump versions, or tag releases.** Bring a
clean, tested, in-scope change with its e2e reading and the maintainers handle
the bookkeeping on merge.

## Security and dependencies

- No hardcoded secrets or credentials, and none in logs or error messages.
- No `eval`/`exec` of dynamic input. Spawn subprocesses with explicit argument
  lists, never by interpolating into a shell string.
- No new network calls without prior discussion.
- Prefer the standard library. Open a Discussion before adding a dependency;
  a new third-party module in a PR is a decision for the maintainers, not a
  detail of the change.

## Commit and PR conventions

- Conventional-style subjects: `feat(...)`, `fix(...)`, `refactor(...)`,
  `docs(...)`, `test(...)`.
- One logical change per PR. Split unrelated work.
- No secrets or credentials, in code, in logs, in error messages, or in pasted
  e2e output. Redact serial numbers and hostnames if you would rather not
  publish them.
- Reference a related issue in the commit body with `refs #<number>`.
- Describe what changed, why, and how it was verified.

## Questions

Open a GitHub Discussion. Issues are for reproducible bug reports and
maintainer-created work items.

---

This policy is shared across the project author's public repositories; the
canonical copy lives in a private sysadmin repository and is rendered into each
project. Project-specific sections (scope, setup, checks, e2e) differ per repo;
the governance sections do not.
