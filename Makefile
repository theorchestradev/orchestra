WAILS ?= $(shell command -v wails 2>/dev/null || echo $(shell go env GOPATH)/bin/wails)
# The Wails CLI must be at least the runtime version in go.mod. Older CLIs bundle
# golang.org/x/tools versions that cannot load standard library types with newer Go.
WAILS_MIN_VERSION := $(shell go list -m -f '{{.Version}}' github.com/wailsapp/wails/v2)
BUILD_WORKTREE := $(shell basename "$$(git rev-parse --show-toplevel 2>/dev/null)" 2>/dev/null)
BUILD_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null)
BUILD_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null)$(shell test -n "$$(git status --porcelain 2>/dev/null)" && echo -dirty)
# Release builds pass the tag without its v prefix, e.g. make build VERSION=0.2.0.
VERSION ?= dev
LDFLAGS := -X main.version=$(VERSION) -X main.buildWorktree=$(or $(BUILD_WORKTREE),dev) -X main.buildBranch=$(BUILD_BRANCH) -X main.buildCommit=$(or $(BUILD_COMMIT),unknown)
# Only the official release workflow enables this; local builds ignore PostHog env vars.
ifeq ($(RELEASE_TELEMETRY),1)
LDFLAGS += -X main.buildTelemetryKey=$(POSTHOG_PROJECT_TOKEN) -X main.buildTelemetryHost=$(or $(POSTHOG_HOST),https://eu.i.posthog.com)
endif

ifeq ($(shell uname -s),Darwin)
DEVELOPER_DIR ?= /Applications/Xcode.app/Contents/Developer
# Keep in sync with LSMinimumSystemVersion in build/darwin/Info.plist. It must be at
# least the minimum macOS version supported by the Go toolchain.
MACOS_MIN_VERSION := 13.0
# cmd/go adds -lobjc for every package with Objective-C files (Wails and
# internal/appearance and internal/apps), so the linker would warn about duplicate libraries.
LDFLAGS += -extldflags=-Wl,-no_warn_duplicate_libraries

export DEVELOPER_DIR
# Wails adds -mmacosx-version-min=10.13 unless these flags already set a target.
# Start from `go env` so the default -O2 -g flags are kept.
export CGO_CFLAGS := $(strip $(shell go env CGO_CFLAGS) -mmacosx-version-min=$(MACOS_MIN_VERSION))
export CGO_LDFLAGS := $(strip $(shell go env CGO_LDFLAGS) -mmacosx-version-min=$(MACOS_MIN_VERSION))
else
# Linux builds against WebKitGTK 4.1 (libwebkit2gtk-4.1-dev), the version current
# Ubuntu LTS releases ship.
TAGS := -tags webkit2_41
endif

.PHONY: build check-wails

build: check-wails
	$(WAILS) build -skipbindings $(TAGS) $(if $(PLATFORM),-platform $(PLATFORM)) -o orchestra -ldflags "$(LDFLAGS)"
ifeq ($(shell uname -s),Darwin)
ifneq ($(VERSION),dev)
# Wails writes productVersion from wails.json; the bundle takes the release version instead.
# CFBundleVersion must be numeric, so it drops a pre-release suffix such as -beta.1.
	plutil -replace CFBundleShortVersionString -string $(VERSION) build/bin/Orchestra.app/Contents/Info.plist
	plutil -replace CFBundleVersion -string $(firstword $(subst -, ,$(VERSION))) build/bin/Orchestra.app/Contents/Info.plist
endif
endif

check-wails:
	@version=$$("$(WAILS)" version 2>/dev/null | grep -o -m1 'v[0-9][0-9.]*'); \
	if [ -z "$$version" ]; then \
		echo "Wails CLI not found at $(WAILS). Install it with:" >&2; \
		echo "  go install github.com/wailsapp/wails/v2/cmd/wails@$(WAILS_MIN_VERSION)" >&2; \
		exit 1; \
	fi; \
	if [ "$$(printf '%s\n' "$(WAILS_MIN_VERSION)" "$$version" | sort -V | head -1)" != "$(WAILS_MIN_VERSION)" ]; then \
		echo "Wails CLI $$version is older than $(WAILS_MIN_VERSION). Upgrade it with:" >&2; \
		echo "  go install github.com/wailsapp/wails/v2/cmd/wails@$(WAILS_MIN_VERSION)" >&2; \
		exit 1; \
	fi
