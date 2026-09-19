default: help

.PHONY: help
help: ## Show this help
	@echo
	@echo "Available commands:"
	@echo
	@awk -F ':|##' '/^[^\t].+?:.*?##/ {printf "\033[36m%-30s\033[0m %s\n", $$1, $$NF}' $(MAKEFILE_LIST)

BINDIR=$(shell go env GOPATH)
MODULE=github.com/ushineko/clockwork-orange
VERSION?=$(shell tr -d 'v[:space:]' < .tag 2>/dev/null || echo dev)
COMMIT?=$(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS=-w -s -X $(MODULE)/internal/buildinfo.Version=$(VERSION) -X $(MODULE)/internal/buildinfo.Commit=$(COMMIT)

# migrated_fynedo tells Fyne this front end has been through the fyne.Do
# migration, so it stops asking which goroutine it is on. Without it, Fyne
# answers that question with runtime.Stack -- a full traceback -- on every
# Canvas.Refresh. Profiled on a sibling program during a window drag: 52% of the
# process's CPU was printing tracebacks. Every UI mutation off the main
# goroutine here goes through fyne.Do, which is what the tag asserts.
# See fynedesygn docs/fyne-quirks.md, quirk 31.
FYNE_TAGS?=migrated_fynedo

LINT_NAME?=golangci-lint
LINT_VERSION?=v2.12.2
LINT_PROGRAM=$(LINT_NAME)-$(LINT_VERSION)

# Release asset coordinates for the pinned linter version.
# (The upstream install.sh is not used: its checksum extraction matches the
# .sbom.json asset line and fails verification on recent releases.)
LINT_VERSION_NUM=$(LINT_VERSION:v%=%)
LINT_BASE_URL=https://github.com/golangci/golangci-lint/releases/download/$(LINT_VERSION)

.PHONY: install-lint
install-lint: $(BINDIR)/bin/$(LINT_PROGRAM) ## Install linter

$(BINDIR)/bin/$(LINT_PROGRAM):
	@echo "Setting up $(LINT_PROGRAM) ..."
	@set -e; \
	os=$$(uname -s | tr '[:upper:]' '[:lower:]'); \
	arch=$$(uname -m); \
	case "$$arch" in x86_64) arch=amd64;; aarch64|arm64) arch=arm64;; esac; \
	dist="$(LINT_NAME)-$(LINT_VERSION_NUM)-$$os-$$arch"; \
	tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; \
	curl -fsSL "$(LINT_BASE_URL)/$$dist.tar.gz" -o "$$tmp/$$dist.tar.gz"; \
	curl -fsSL "$(LINT_BASE_URL)/$(LINT_NAME)-$(LINT_VERSION_NUM)-checksums.txt" -o "$$tmp/checksums.txt"; \
	want=$$(awk -v f="$$dist.tar.gz" '$$2 == f {print $$1}' "$$tmp/checksums.txt"); \
	got=$$( (sha256sum "$$tmp/$$dist.tar.gz" 2>/dev/null || shasum -a 256 "$$tmp/$$dist.tar.gz") | awk '{print $$1}'); \
	if [ -z "$$want" ] || [ "$$want" != "$$got" ]; then echo "checksum mismatch for $$dist.tar.gz: want '$$want' got '$$got'"; exit 1; fi; \
	tar -C "$$tmp" -xzf "$$tmp/$$dist.tar.gz"; \
	mkdir -p "$(BINDIR)/bin"; \
	mv -v "$$tmp/$$dist/$(LINT_NAME)" "$(BINDIR)/bin/$(LINT_PROGRAM)"

.PHONY: setup
setup: install-lint ## Setup system for local development
	@echo "Make sure your system path includes GOPATH/bin. See README.md for details."

# golangci-lint type-checks against the standard library sources of whichever Go
# it finds, using a go/types built into the linter binary. A linter built with
# Go 1.26 panics outright ("file requires newer Go version go1.27") on a machine
# whose GOROOT is 1.27. go.mod deliberately carries no `toolchain` line (R1.1),
# so the pin lives here instead, matching the Go that this linter release was
# built with. Bump it together with LINT_VERSION.
LINT_GO_TOOLCHAIN?=go1.26.0

.PHONY: lint
lint: export GOTOOLCHAIN = $(LINT_GO_TOOLCHAIN)
lint: install-lint ## Lint files
	@go version
	$(BINDIR)/bin/$(LINT_PROGRAM) run --timeout 5m0s --config config/.golangci-$(LINT_VERSION).yml ./...

# The parity test carries a build tag so that a plain `go test ./...` does not
# need it, and `make test` always does: it is the guard that the CLI and the GUI
# expose the same operations, and a guard that runs only when someone remembers
# to ask for it is not a guard. It needs no display.
.PHONY: test
test: ## Run unit tests with the race detector (fast, no build)
	@go test -race -tags parity ./...

.PHONY: coverage
coverage: ## Run all tests and open a coverage report in the default browser
	@go test -coverprofile coverage.out ./...
	@go tool cover -html=coverage.out
	@rm -f coverage.out

.PHONY: build
build: ## Build the CLI/daemon for the host platform (no CGO: the daemon needs no display)
	@mkdir -p bin
	CGO_ENABLED=0 go build -ldflags='$(LDFLAGS)' -trimpath -o bin/clockwork-orange ./cmd/clockwork-orange

.PHONY: build-gui
build-gui: ## Build the desktop front end for the host platform (requires CGO)
	@mkdir -p bin
	CGO_ENABLED=1 go build -tags $(FYNE_TAGS) -ldflags='$(LDFLAGS)' -trimpath -o bin/clockwork-orange-gui ./cmd/clockwork-orange-gui

.PHONY: build-all
build-all: ## Build static CLI binaries for every platform, plus the host's GUI
	@set -e; for p in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do \
		os=$${p%/*}; arch=$${p#*/}; \
		echo "building $$os/$$arch ..."; \
		mkdir -p dist; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
			go build -ldflags='$(LDFLAGS)' -trimpath -o dist/clockwork-orange-$$os-$$arch ./cmd/clockwork-orange; \
	done
	@# The GUI is built for this machine only. It needs CGO, so cross-compiling
	@# it would need a C toolchain per target; CI builds it on a runner per OS
	@# (spec 010 R8.6). The daemon works without it.
	@echo "building the GUI for this host ..."; \
	if CGO_ENABLED=1 go build -tags $(FYNE_TAGS) -ldflags='$(LDFLAGS)' -trimpath \
		-o dist/clockwork-orange-gui-$$(go env GOOS)-$$(go env GOARCH) ./cmd/clockwork-orange-gui; then \
		echo "  built dist/clockwork-orange-gui-$$(go env GOOS)-$$(go env GOARCH)"; \
	else \
		echo "  the GUI did not build; the CLI binaries are unaffected" >&2; \
	fi
	@ls -l dist/

.PHONY: install
install: ## Install the CLI, the GUI, the desktop entry and the icon into ~/.local
	./install.sh

.PHONY: uninstall
uninstall: ## Remove what install put in ~/.local, leaving your data alone
	./uninstall.sh

# One tarball per target plus a checksum file, which is what a release page
# needs and what `curl | sha256sum -c` checks. The GUI is host-only (it needs
# CGO), so it is packaged only for the platform it was built on.
.PHONY: release
release: build-all ## Package dist/ into per-target tar.gz archives with SHA256SUMS
	@set -e; 	rm -f dist/*.tar.gz dist/SHA256SUMS; 	stage=$$(mktemp -d); trap 'rm -rf "$$stage"' EXIT; 	for bin in dist/clockwork-orange-*; do 		case "$$bin" in *.tar.gz|*SHA256SUMS) continue;; esac; 		name=$$(basename "$$bin"); 		target=$${name#clockwork-orange-}; target=$${target#gui-}; 		dir="$$stage/clockwork-orange-$(VERSION)-$$target"; 		mkdir -p "$$dir"; 		case "$$name" in 			clockwork-orange-gui-*) cp "$$bin" "$$dir/clockwork-orange-gui";; 			*) cp "$$bin" "$$dir/clockwork-orange";; 		esac; 		cp README.md LICENSE "$$dir/"; 		mkdir -p "$$dir/packaging"; 		cp packaging/io.ushineko.clockwork-orange.desktop packaging/clockwork-orange.png "$$dir/packaging/"; 	done; 	for dir in "$$stage"/*; do 		base=$$(basename "$$dir"); 		tar -C "$$stage" -czf "dist/$$base.tar.gz" "$$base"; 	done; 	cd dist && (sha256sum *.tar.gz 2>/dev/null || shasum -a 256 *.tar.gz) > SHA256SUMS
	@ls -l dist/*.tar.gz dist/SHA256SUMS

# The Arch package, built from this checkout by packaging/arch/PKGBUILD. The
# same target GitHub Actions runs; the result lands beside the PKGBUILD.
.PHONY: pkg-deb
pkg-deb: build build-gui ## Build the Debian package from this checkout (needs dpkg-deb)
	./packaging/debian/build.sh

.PHONY: pkg-arch
pkg-arch: ## Build the Arch Linux package from this checkout (needs makepkg)
	@# BUILDDIR outside the tree: makepkg's pkg/ is unreadable to `go test ./...`
	@# and has no business inside a Go module.
	cd packaging/arch && BUILDDIR="$$(mktemp -d)" makepkg -sf --noconfirm
	@ls -l packaging/arch/*.pkg.tar.zst

.PHONY: clean
clean: ## Remove build artifacts
	@rm -rf bin/ dist/ coverage.out count.out packaging/arch/pkg packaging/arch/src packaging/arch/*.pkg.tar.zst
