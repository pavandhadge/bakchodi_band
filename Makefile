APP := bakchodi
MODULE := ./cmd
DIST_DIR := dist
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GO ?= go
GOCACHE ?= /tmp/go-build-cache
GOMODCACHE ?= /tmp/go-mod-cache
GOFLAGS ?= -trimpath
LDFLAGS ?= -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

.PHONY: help test build clean release-build checksums package release-check release publish install-local

help:
	@echo "Targets:"
	@echo "  make test                  Run Go tests"
	@echo "  make build                 Build local ./$(APP)"
	@echo "  make release-build         Build release binaries into $(DIST_DIR)/"
	@echo "  make package               Build release binaries and checksums"
	@echo "  make release-check         Run checks before publishing"
	@echo "  make publish VERSION=v0.1.0 Publish GitHub release with gh"
	@echo "  make release VERSION=v0.1.0 Run checks, package, then publish"
	@echo "  make clean                 Remove local build outputs"

test:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) test ./...

build:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(APP) $(MODULE)

clean:
	rm -rf $(DIST_DIR) $(APP) $(APP).exe

release-build: clean
	mkdir -p $(DIST_DIR)
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		out="$(DIST_DIR)/$(APP)-$$os-$$arch"; \
		if [ "$$os" = "windows" ]; then out="$$out.exe"; fi; \
		echo "building $$out"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o "$$out" $(MODULE); \
	done

checksums: release-build
	cd $(DIST_DIR) && sha256sum $(APP)-* > SHA256SUMS

package: checksums
	@echo "Release assets are ready in $(DIST_DIR)/"

release-check:
	test -n "$(VERSION)"
	@case "$(VERSION)" in v*) true ;; *) echo "VERSION must look like v0.1.0"; exit 1 ;; esac
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) test ./...
	sh -n packaging/install.sh
	sh -n packaging/install-release.sh
	git diff --check

publish:
	@case "$(VERSION)" in v*) true ;; *) echo "Set a release tag: make publish VERSION=v0.1.0"; exit 1 ;; esac
	@command -v gh >/dev/null 2>&1 || { echo "GitHub CLI required: install gh and run gh auth login"; exit 1; }
	@test -d $(DIST_DIR) || { echo "Run make package VERSION=$(VERSION) first"; exit 1; }
	gh release create "$(VERSION)" $(DIST_DIR)/$(APP)-* $(DIST_DIR)/SHA256SUMS \
		--title "$(VERSION)" \
		--notes "Release $(VERSION)"

release: release-check package publish

install-local: build
	@case "$$(uname -s)" in \
		Linux) sudo sh packaging/linux/install.sh ./$(APP) ;; \
		Darwin) sudo sh packaging/macos/install.sh ./$(APP) ;; \
		*) echo "Unsupported local install OS"; exit 1 ;; \
	esac
