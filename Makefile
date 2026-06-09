# VLESS Client build targets.
#
# Version flows from the git tag (override with `make VER=v0.1.0 <target>`).
# Build scripts live in scripts/; this Makefile is a thin, discoverable wrapper.

VER     ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VER)
GUI_TAGS := wails webkit2_41

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
	  | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

## --- Dev / test ---

.PHONY: test
test: ## Run the full Go test suite
	go test ./...

.PHONY: headless
headless: ## Build the headless CLI (no GUI, no CGO)
	go build -ldflags "$(LDFLAGS)" -o build/bin/headless ./cmd/headless

.PHONY: dev
dev: ## Run the Wails GUI in live-reload dev mode (Linux)
	wails dev -tags "$(GUI_TAGS)" -ldflags "$(LDFLAGS)"

.PHONY: gui
gui: ## Build the Wails GUI for the host OS (Linux)
	wails build -tags "$(GUI_TAGS)" -ldflags "$(LDFLAGS)"

## --- Release builds (per OS) ---

.PHONY: windows
windows: ## Cross-build the Windows NSIS installer (needs mingw + nsis)
	VER=$(VER) scripts/build-windows.sh

.PHONY: macos
macos: ## Build, sign, notarize the macOS .dmg (run on a Mac, needs APPLE_* env)
	VER=$(VER) scripts/build-macos.sh

.PHONY: release
release: ## Build the release artifact for this host OS (Linux->Windows, Mac->macOS)
	VER=$(VER) scripts/release.sh

.PHONY: tag
tag: ## Create the git tag for the current VER (e.g. make VER=v0.1.0 tag)
	git tag $(VER)

.PHONY: publish
publish: ## Publish build/release/* to a GitHub release for VER (run after builds + QA)
	gh release create $(VER) build/release/* \
	  --title "VLESS Client $(VER)" --notes-file CHANGELOG.md

## --- Assets / cleanup ---

.PHONY: icon
icon: ## Regenerate the Windows .ico from build/appicon.png (needs ImageMagick)
	convert build/appicon.png -define icon:auto-resize=256,128,64,48,32,16 build/windows/icon.ico
	@echo "icon.ico regenerated; macOS .icns is auto-generated from appicon.png at build time"

.PHONY: clean
clean: ## Remove build outputs
	rm -rf build/bin build/release
