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
just test-one pkg/manager TestInstall
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

The codebase uses a layered design: stateless Backend adapters → Manager orchestration → independent utility packages.

### Package Structure

```
cmd/           CLI parsing, user-facing output, wiring
pkg/backend/   Backend interface and implementations (github, shasumurl, kubeurl)
pkg/manager/   Orchestration: install/update/status/list/uninstall lifecycle
pkg/manifest/  Manifest schema (Package/InstallSpec), storage, and loading
pkg/fetch/     HTTP downloading with progress reporting
pkg/extract/   In-memory archive decompression and file extraction
pkg/verify/    Checksum computation and verification
```

### Command Structure (cmd/)

Uses Cobra for CLI commands. Each command is in its own file:
- `root.go` — Entry point, wires Manager with all dependencies via `manager.New()`
- `install.go` — Parses `URL[@VERSION]`, `--file`, `--checksum`, `--dir`, `--type`, `--pin`
- `update.go` — Updates packages; `--pin`/`--unpin` flags
- `status.go` — Checks for updates; exits 1 if any update available
- `list.go` — Lists all installed packages with versions and file paths
- `uninstall.go` — Removes installed packages

### Backend Package (pkg/backend/)

Each backend implements the `Backend` interface:

```go
type Backend interface {
    Resolve(ctx context.Context, sourceURL *url.URL, opts ResolveOptions) (*Resolution, error)
    Check(ctx context.Context, pkg *manifest.Package) (*Resolution, error)
    Type() string
    CanHandle(u *url.URL) bool
}
```

**Backend Types:**
- `github.go` — GitHub releases; auto-detected for `github.com` URLs; plain net/http with Bearer token from `~/.config/gh/hosts.yml`
- `shasumurl.go` — Generic URLs with sha256sum.txt files; version = SHA-256 of file content; `CanHandle` always false (must use `--type shasumurl`)
- `kube.go` — Kubernetes releases from `dl.k8s.io`; auto-detected; fetches version from stable pointer URL; `Resolution.Assets` is nil (manager constructs download URLs)
- `registry.go` — Dispatches to backends by URL or explicit `--type`

**Resolution type:**
```go
type Resolution struct {
    Version string
    Assets  []Asset  // nil for kubeurl; manager constructs URLs directly
}
type Asset struct {
    Name, URL  string
    Checksums  map[string]string  // pre-populated by shasumurl backend
}
```

### Manager Package (pkg/manager/)

Single orchestrator; no layer calls another directly:
- `manager.go` — `Manager` interface and `New()` constructor
- `install.go` — Full install flow: resolve → select assets → fetch → verify → extract → write → save manifest
- `update.go` — Parallel Check via WaitGroup; re-installs at new version
- `status.go` — Parallel Check; returns `[]*StatusResult`
- `checksum.go` — Strategy resolution: `auto`, `shared-file`, `per-asset`, `multisum`, `embedded`, `none`; parsers for sha256sums (text and binary mode) and multisum formats
- `vars.go` — `ExpandVars(pattern, tag)`: substitutes `${TAG}` and `${VERSION}`

### Manifest Package (pkg/manifest/)

- `package.go` — `Package`, `InstallSpec`, `ChecksumConfig`, `DownloadedAsset`, `InstalledFile` types with JSON tags
- `store.go` — `Save`, `Load`, `LoadAll`, `Delete` (all take `dir string`; fall back to `LibDir()` when empty); `LibDir()` = `~/.local/share/binmgr/`

### Data Flow (Install)

1. Parse URL → dispatch to backend via registry
2. `Resolve()` → `Resolution{Version, Assets}`
3. Match `--file` ASSET_GLOB against assets
4. Expand `${TAG}`/`${VERSION}` in traversal globs and local names
5. Fetch asset bytes (in-memory)
6. Resolve checksums per strategy (fetch checksum file if needed)
7. `Verify()` asset bytes against expected checksums
8. `Extract()` traversal globs from archive (in-memory)
9. Write files to disk at 0755
10. Save `manifest.Package` to `~/.local/share/binmgr/`

### Checksum Strategies (`--checksum`)

| Strategy | Description |
|----------|-------------|
| `auto` | Search resolution assets for `SHA256SUMS`, `SHA256SUMS.txt`, `checksums.txt`, `checksums.sha256`, `*checksums*`, `*sha256*` |
| `shared-file:GLOB` | Fetch named asset, parse as sha256sums, look up by filename |
| `per-asset:SUFFIX` | Fetch `{assetURL}{suffix}`, parse as hex or sha256sums-format line |
| `multisum[:DATA[:ORDER]]` | HashiCorp multisum format with separate data and order files |
| `embedded:GLOB` | Extract checksum file from inside the archive |
| `none` | Skip verification |

sha256sums parser accepts both text mode (`hex  filename`) and binary mode (`hex *filename`).
Verifier skips unknown hash algorithms; fails only on mismatches for algorithms it can compute.

### Variable Substitution

In `--file` specs and checksum file globs:
- `${TAG}` — raw release tag (e.g. `v1.40.0`)
- `${VERSION}` — tag with leading `v` stripped (e.g. `1.40.0`)

### URL Auto-Detection

- `github.com/*` → github backend
- `dl.k8s.io/*` → kubeurl backend
- Others → require `--type shasumurl` (or other future backend types)

## Development Notes

- Logging uses apex/log with configurable levels via `--loglevel` flag (default: warn)
- Progress bars use schollz/progressbar for download feedback
- All operations use a 5-minute context timeout
- GitHub auth: reads Bearer token from `~/.config/gh/hosts.yml`; falls back to unauthenticated
