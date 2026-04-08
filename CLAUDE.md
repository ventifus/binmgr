# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

binmgr is a Go CLI tool that installs and manages binaries from various sources (GitHub releases, arbitrary URLs with checksums, Kubernetes releases). It tracks installed binaries with manifest files and can check for/install updates.

## Build and Test Commands

This project uses [just](https://github.com/casey/just) as a command runner for all development tasks. Use `just` for building, testing, and code quality checks.

**Common commands:**

Build the binary:
```bash
just build
```

Run all tests:
```bash
just test
```

Run tests with coverage:
```bash
just test-cover
```

Test specific package:
```bash
just test-pkg pkg/backend
```

Run specific test:
```bash
just test-one pkg/backend TestNewBinmgrManifest
```

Generate HTML coverage report:
```bash
just coverage-html
```

Format code:
```bash
just fmt
```

Run all quality checks (fmt, vet, test):
```bash
just check
```

Pre-commit checks (fmt, vet, test with coverage):
```bash
just pre-commit
```

Clean build artifacts:
```bash
just clean
```

**See all available commands:**
```bash
just --list
```

**Direct Go commands (use just instead):**
- `just build` → `go build -o binmgr ./cmd/binmgr/...`
- `just test` → `go test ./...`
- `just fmt` → `go fmt ./...`
- `just vet` → `go vet ./...`

## Architecture

### Command Structure (cmd/)

Uses Cobra for CLI commands. Each command is in its own file:
- `root.go` - Entry point, config initialization, sets up viper to read from `~/.binmgr.yaml`
- `install.go` - Installs binaries, auto-detects backend type from URL
- `update.go` - Updates installed binaries to latest versions
- `status.go` - Checks if installed binaries have updates available
- `list.go` - Lists all installed binaries
- `uninstall.go` - Removes installed binaries

### Backend Package (pkg/backend/)

Core logic for different package sources. Each backend type implements install/update/status operations:

**Backend Types:**
- `github.go` - GitHub releases (auto-detected for github.com URLs)
- `shasumurl.go` - Generic URLs with SHA256SUMS files
- `kube.go` - Kubernetes releases from dl.k8s.io (auto-detected)

**Supporting Files:**
- `manifest.go` - Defines `BinmgrManifest` struct that tracks installed binaries. Manifests are stored as JSON in `~/.local/share/binmgr/`
- `installedfile.go` - Handles file downloads, extraction (tar.gz, zip, bzip2), and installation to target location
- `shasums.go` - Checksum verification logic supporting multiple formats (sha256sums, per-asset checksums, multisum)
- `url.go` - URL utilities

### Manifest System

Each installed binary has a manifest JSON file in `~/.local/share/binmgr/` containing:
- Package name and type (github/shasumurl/kubeurl)
- Current version and remote URL
- List of artifacts (downloaded files) with checksums
- Original command line used for installation (for updates)

This allows `update` to re-run the original install with new versions and `status` to check for newer releases.

### File Installation Flow

1. Backend determines available assets/files from source
2. User-specified glob pattern filters to desired file
3. File is downloaded and checksum verified
4. If archived (tar.gz, zip, bzip2), extract inner files
5. Install to target location (default: `~/.local/bin/`)
6. Save manifest for future updates

### Checksum Verification

Supports multiple checksum formats:
- `sha256sums` - Standard SHA256SUMS file
- `per_asset:sha256sum` - Individual .sha256sum files per asset
- `per_asset:sha256` - Individual .sha256 files per asset
- `multisum` - Files containing checksums with algorithm prefixes (e.g., "sha-256:")
- `none` - Skip verification (not recommended)

### URL Auto-Detection

`install.go` automatically sets backend type based on URL:
- `github.com/*` → github backend
- `dl.k8s.io/*` → kubeurl backend
- Others → use `--type` flag to specify

### GitHub Authentication

Attempts to use GitHub CLI (`gh`) config from `~/.config/gh/hosts.yml` for authenticated API requests (higher rate limits). Falls back to unauthenticated if not available.

## Development Notes

- Logging uses apex/log with configurable levels via `--loglevel` flag (default: warn)
- Configuration via viper reads from `~/.binmgr.yaml` if present
- Progress bars use schollz/progressbar for download feedback
- All backend operations use context with 5-minute timeout
