# binmgr Specification

## Purpose

binmgr is a CLI tool that installs and manages binaries from external sources. It fills a gap between OS package managers (which only know about packaged software) and manually downloading binaries (which requires the user to track versions, checksums, and update paths).

The core value proposition: install a binary from any supported source with a single command, and later update it to the latest version with a single command.

## Concepts

**Package** — a named, versioned piece of software that binmgr knows how to fetch. A package comes from a specific source (a backend), has a current version, and can be updated to a newer version.

**Backend** — an adapter for a specific type of package source. Each backend knows how to discover available versions, resolve download URLs, and detect when an update is available. Backends are interchangeable from the user's perspective.

**Asset** — a downloadable file at a package source. A release on GitHub may have many assets (one per OS/architecture combination). An asset may be a binary or an archive containing one or more binaries.

**Install spec** — a declaration of one file or set of files to install from a package. A package may have multiple install specs, each independently selecting an asset, navigating any archive layers, and specifying a local name. All specs for a package are tracked together under one manifest.

**Artifact** — a file that binmgr tracks. Each install spec produces one or more artifacts: the downloaded file itself, and any files extracted from it if it is an archive. Checksums are tracked at every level.

**Manifest** — the local record of an installed package. Stores enough information to check for updates, verify integrity, and re-install.

## Backends

The backend is selected automatically based on the source URL, or specified explicitly with `--type`.

### `github` — GitHub Releases

Installs from GitHub release assets. The version is the release tag.

Releases can be targeted by:
- **Latest**: the most recent published release (default)
- **Tag**: a specific git tag (e.g., `v1.24.0`), for reproducible or pinned installs
- **ID**: a numeric GitHub release ID, stable even if the tag is later changed

Asset and checksum file glob patterns support `${VERSION}` and `${TAG}` variable substitution (see [Variable Substitution](#variable-substitution)).

If the GitHub CLI (`gh`) is configured on the machine, binmgr uses its stored credentials to make authenticated API requests. This enables access to private repositories and avoids public rate limits.

### `shasumurl` — OpenShift Mirror

Installs from OpenShift mirror releases, which publish a `sha256sum.txt` file listing all available artifacts and their checksums. binmgr uses a glob pattern to select which file(s) to install, then resolves download URLs relative to the checksum file's location.

Versioning is content-based: binmgr hashes the checksum file's full content (SHA-256) and stores that digest as the package `version`. An update is detected when the hash changes on the next `status` or `update` check. There is no semantic version; the stored hash is what is displayed in `list` and `status` output.

Checksums for downloaded assets are provided directly by the backend from the parsed checksum file — no separate `--checksum` strategy needs to be specified by the user. The `DownloadedAsset.checksums` field is always populated for shasumurl packages.

### `kubeurl` — Kubernetes Release Infrastructure

Installs binaries from `dl.k8s.io` using Kubernetes's own release infrastructure. The stable version is read from a canonical `stable.txt` pointer file; the binary and its per-asset checksum are fetched from URLs that embed the version.

This backend handles Kubernetes's version-discovery convention specifically, so users provide a URL pattern rather than a fully-resolved URL.

## Examples

The following concrete cases, drawn from real installed packages, illustrate the full range of patterns binmgr must support. Subsequent sections reference these by label.

### E1 — Direct binary with rename

**Repo:** https://github.com/knative/func  
**Asset:** `func_linux_amd64`  
**Local name:** `kn-func`  
**Checksum:** sha256 (inline on asset)

The asset name is platform-specific and meaningless locally. An explicit local name is required. Illustrates: [asset selection](#asset-selection), [local naming](#local-naming).

### E2 — Gzip-compressed single binary

**Repo:** https://github.com/tree-sitter/tree-sitter  
**Asset:** `tree-sitter-linux-x64.gz`  
**Local name:** `tree-sitter`  
**Checksum:** none

A `.gz` that decompresses to a single bare executable — not a tarball. No traversal needed. Illustrates: [archive traversal](#archive-traversal), [no checksum](#checksum-source-strategies).

### E3 — Tar.gz with basename traversal and static checksum filename

**Repo:** https://github.com/casey/just  
**Asset:** `just-${VERSION}-x86_64-unknown-linux-musl.tar.gz`  
**Traversal:** `just`  
**Local name:** `just` (default basename)  
**Checksum:** shared file — `SHA256SUMS`

Traversal glob matches by basename regardless of directory depth inside the archive. The checksum file has a fixed name. Illustrates: [asset selection](#asset-selection), [archive traversal](#archive-traversal), [version templating](#version-templating), [shared release file](#checksum-source-strategies).

### E4 — Tar.gz with versioned path traversal and versioned checksum filename

**Repo:** https://github.com/cli/cli  
**Asset:** `gh_${VERSION}_linux_amd64.tar.gz`  
**Traversal:** `gh_${VERSION}_linux_amd64/bin/gh`  
**Local name:** `gh` (default basename)  
**Checksum:** shared file — `gh_${VERSION}_checksums.txt`

The traversal glob includes a version placeholder, which must be stored unexpanded; if expanded at install time and stored literally, the path would not match on update. Both the asset and checksum filename use `${VERSION}`. Illustrates: [archive traversal](#archive-traversal), [version templating](#version-templating), [shared release file](#checksum-source-strategies).

### E5 — Zip with path traversal and per-asset checksum

**Repo:** https://github.com/Azure/kubelogin  
**Asset:** `kubelogin-linux-amd64.zip`  
**Traversal:** `bin/linux_amd64/kubelogin`  
**Local name:** `kubelogin`  
**Checksum:** per-asset — `.sha256` suffix

Zip archive with a multi-level path to the binary inside. Per-asset checksum file co-located with the asset. Illustrates: [archive traversal](#archive-traversal), [local naming](#local-naming), [per-asset files](#checksum-source-strategies).

### E6 — Bzip2 tarball

**Repo:** https://github.com/aristocratos/btop  
**Asset:** `btop-x86_64-linux-musl.tbz`  
**Traversal:** `btop`  
**Local name:** `btop` (default basename)  
**Checksum:** none

`.tbz` is bzip2-compressed tar. Same traversal model as gzip tar; the compression format is detected automatically. Illustrates: [archive traversal](#archive-traversal).

### E7 — Multiple files from one archive to different directories

**Repo:** https://github.com/git-ecosystem/git-credential-manager  
**Asset:** `gcm-linux_amd64.2.6.1.tar.gz`  
**Spec 1:** traversal `git-credential-manager` → `~/.local/bin/git-credential-manager`  
**Spec 2:** traversal `libHarfBuzzSharp.so` → `~/.local/lib/libHarfBuzzSharp.so`  
**Checksum:** none

One archive, two install specs, two different destination directories. Currently requires hand-editing the manifest JSON. Illustrates: [install specs](#install-specs), [local naming](#local-naming) (absolute path overrides directory).

### E8 — Direct binary, no checksum

**Repo:** https://github.com/da-luce/astroterm  
**Asset:** `astroterm-linux-x86_64`  
**Local name:** `astroterm`  
**Checksum:** none (project publishes no checksums)

Rename required; no checksum available and must be declared explicitly. Illustrates: [local naming](#local-naming), [no checksum](#checksum-source-strategies).

### E9 — Versioned checksum filename

**Repo:** https://github.com/aquasecurity/trivy  
**Asset:** `*Linux-64bit.tar.gz`  
**Traversal:** `trivy`  
**Local name:** `trivy` (default basename)  
**Checksum:** shared file — `trivy_${VERSION}_checksums.txt`

The checksum file's name embeds the version and must use `${VERSION}` substitution to locate it. Illustrates: [version templating](#version-templating), [shared release file](#checksum-source-strategies).

### E10 — Multisum

**Repo:** https://github.com/mikefarah/yq  
**Asset:** `yq_linux_amd64.tar.gz`  
**Traversal:** `./yq_linux_amd64`  
**Local name:** `yq`  
**Checksum:** multisum — data file `checksums`, ordering file `checksums_hashes_order`

Multiple hash algorithms verified simultaneously. The traversal glob includes a leading `./` as published in the archive. Illustrates: [archive traversal](#archive-traversal), [multisum](#checksum-source-strategies).

### E11 — Kubernetes version pointer

**Source:** `https://dl.k8s.io/release/stable.txt` (version pointer) → `https://dl.k8s.io/{VERSION}/bin/linux/amd64/kubectl`  
**Local name:** `kubectl`  
**Checksum:** per-asset — `.sha256` suffix on the binary URL

Version is not a release tag but a string read from `stable.txt`. Checksum fetched from a predictable URL adjacent to the binary. Illustrates: [`kubeurl` backend](#kubeurl--kubernetes-release-infrastructure), [per-asset files](#checksum-source-strategies).

### E12 — OpenShift mirror

**Source:** `https://mirror.openshift.com/pub/openshift-v4/x86_64/clients/ocp/stable/sha256sum.txt`  
**Asset glob:** `openshift-client-linux-amd64-rhel9-*`  
**Traversal:** `oc`  
**Local name:** `oc` (default basename)  
**Checksum:** embedded in the shasumurl checksum file (the source file serves as both version marker and checksum source)

No release API. Version detected by comparing checksum file content between runs. Illustrates: [`shasumurl` backend](#shasumurl--openshift-mirror).

### E13 — Non-standard version tag

**Repo (no leading `v`):** https://github.com/casey/just — tag `1.46.0`, so `${TAG}` = `1.46.0`, `${VERSION}` = `1.46.0` (identical; stripping `v` has no effect)  
**Repo (non-`v` prefix):** https://github.com/knative/client — tag `knative-v1.19.5`, so `${TAG}` = `knative-v1.19.5`, `${VERSION}` = `knative-v1.19.5` (stripping leading `v` has no effect on a `k…` prefix)

Not all projects follow the `v1.2.3` convention. `${VERSION}` reliably strips a leading `v` but cannot handle arbitrary prefixes. Globs using `${TAG}` are safer when the tag format is unknown. Illustrates: [version templating](#version-templating).

---

## File Selection

Most package sources offer multiple files per release, and those files may themselves be archives containing further files. File selection is the mechanism for declaring exactly what to download, what to extract, and what to name it locally. The CLI encoding of install specs is defined in [cli.md](cli.md); the manifest schema for stored specs is in [datamodel.md](datamodel.md).

### Install Specs

A package is described by one or more **install specs**. Each spec is an independent declaration of one file (or set of files) to install. A package with multiple specs installs multiple files, all tracked under one manifest. This supports common cases like:

- Installing both `oc` and `kubectl` from the same OpenShift release tarball
- Installing a binary alongside a shared library, each to a different directory (E7)
- Installing multiple independent assets from the same GitHub release

When multiple specs reference the same asset, the asset is downloaded only once.

### Asset Selection

Each spec begins with an **asset glob** that selects which file to download from the source. For GitHub releases, this matches against release asset names (E3, E4, E9). For shasumurl backends, it matches against filenames listed in the checksum file (E12).

Asset globs support `${VERSION}` and `${TAG}` substitution, resolved at install time. The substituted pattern is stored unexpanded in the manifest so updates can re-resolve it against a new version (E3, E4).

### Archive Traversal

If the downloaded file is an archive (tar, zip) or a compressed file (gzip, bzip2), binmgr decompresses and extracts it automatically. Compression layers are stripped progressively: a `.tar.gz` is first decompressed to a tar, then the tar is extracted. A plain `.gz` decompresses to a single file with no further traversal needed (E2). A `.tbz` is bzip2-compressed tar and follows the same model as `.tar.gz` (E6).

A **traversal glob** selects which file(s) to install from within an extracted archive. The glob is matched against both the full path of each entry and its basename — a simple name like `trivy` matches `trivy`, `linux-amd64/trivy`, or `trivy-1.0/bin/trivy` (E3, E9). A path-containing glob like `bin/linux_amd64/kubelogin` matches the full path exactly (E5). Traversal globs may also contain `${VERSION}` placeholders, which must be stored unexpanded for updates to work correctly (E4).

A traversal glob that matches multiple entries installs all of them (E7). If no traversal glob is given and the result is an archive, all files in it are installed.

Multiple traversal levels handle nested archives: each level descends one layer before the next traversal glob is applied.

### Local Naming

Renaming is common and expected. Release assets are typically named with platform suffixes (`kubectl-linux-amd64`, `ha_amd64`, `func_linux_amd64`) that differ from the desired local name. An explicit **local name** overrides the default and decouples what a file is called locally from what it is called at the source (E1, E8).

- For archive-sourced files, the default local name is the basename of the matched path (E3).
- For direct binary downloads, the default local name is the asset name as-is.

A local name may be a bare filename, resolved against the package's default install directory. Or it may be an absolute path, which also overrides the install directory for that specific file. This allows a single package to install files to multiple directories (e.g., a binary to `~/.local/bin/` and a shared library to `~/.local/lib/`) (E7).

### Variable Substitution

Patterns — asset globs, traversal globs, and checksum file globs — support variable substitution using `${VAR}` syntax. Substitution is simple string replacement: no template language, no conditionals, no functions. The available variables are:

| Variable | Value | Example |
|----------|-------|---------|
| `${TAG}` | The raw release tag | `v1.24.0`, `1.46.0`, `knative-v1.19.5` |
| `${VERSION}` | The tag with any leading `v` stripped | `1.24.0`, `1.46.0`, `knative-v1.19.5` |

`${VERSION}` is a convenience for the common `v1.2.3` convention. When a tag does not begin with `v` (E13), `${TAG}` and `${VERSION}` are identical — `${TAG}` is the safer default when the tag format is unknown.

Substitution is applied uniformly: the same variables are available in every pattern, regardless of whether the pattern is selecting an asset, navigating an archive, or locating a checksum file (E3, E4, E9).

Patterns are stored in the manifest in their original unsubstituted form. On update, the backend resolves the new version and substitutes it fresh into all patterns. A pattern that embeds a literal version string rather than a variable will not match the updated release (E4 illustrates this failure mode).

## Checksum Verification

Locating checksums is a selection problem of its own, analogous to file selection. The checksum source must be found before a download can be verified, and different projects publish checksums in fundamentally different ways. The CLI encoding of checksum strategies is defined in [cli.md](cli.md); the manifest schema for stored checksum configuration is in [datamodel.md](datamodel.md).

Checksum verification happens at two points:

1. **Download verification** — before extraction or installation, the downloaded asset is verified against a checksum from the source.
2. **Extraction recording** — after extraction, checksums of all installed files are computed and stored in the manifest for future integrity re-checks, regardless of whether a download-time source was available.

### Checksum Source Strategies

The strategy is specified at install time and stored in the manifest so updates verify consistently. Variable substitution (see [Variable Substitution](#variable-substitution)) applies to any glob patterns used to locate checksum files.

**Shared release file** — a single file at the release level lists checksums for all assets in the format `HASH  FILENAME` (one per line). The file is located by a glob pattern matched against the release assets. The glob typically needs to be specified explicitly because naming varies widely by project: `SHA256SUMS` (E3), `checksums.sha256`, `checksums.txt`, `trivy_${VERSION}_checksums.txt` (E9), `gh_${VERSION}_checksums.txt` (E4).

**Per-asset files** — each asset has a corresponding checksum file adjacent to it, named by a predictable convention (e.g., `{asset}.sha256` or `{asset}.sha256sum`). The naming suffix is specified as part of the strategy. Used by the Kubernetes project (E11) and Azure/kubelogin (E5).

**Multisum** — a HashiCorp-specific format using two co-located files: a data file with multiple hash algorithms per asset (columns), and an ordering file that maps each column to an algorithm. Both files are located by glob. This enables simultaneous verification across multiple algorithms (E10).

**Embedded** — a checksum file located within the downloaded archive itself, identified by a traversal glob (same mechanism as file selection). This covers the extracted files rather than the archive as a whole. Useful for projects that bundle a checksum file alongside their binary inside a tarball.

**None** — no download-time verification. Must be declared explicitly with `--checksum none`; binmgr never silently skips verification (E2, E6, E7, E8).

### Auto-Detection Heuristic

When no checksum strategy is specified, binmgr attempts to find a shared checksum file among the release assets. This is a first-pass heuristic that will need tuning as real-world repos are tested — the names and conventions vary widely in practice.

Initial search order: `SHA256SUMS`, `SHA256SUMS.txt`, `checksums.txt`, `checksums.sha256`, then any asset matching `*checksums*` or `*sha256*`. If a match is found, it is used as a `shared-file` strategy. If no match is found, binmgr prints the names that were tried and exits with an error — it does not proceed without verification. The user must then specify an explicit strategy (including `--checksum none` to opt out intentionally). The search order and glob patterns should be revised as testing reveals gaps.

## Operations

### install

Downloads and installs a binary from a source URL. Records a manifest for future update and status checks.

Parameters:
- Source URL (required) — determines the backend via auto-detection, or use `--type` to override
- One or more install specs — declare what to download, how to navigate any archives, and what to name the installed files (see [File Selection](#file-selection))
- Checksum strategy — how to verify downloads
- Default install directory — where to install files that don't specify an absolute path (default: `~/.local/bin/`)
- Version pin — install a specific version and exclude this package from future updates

The CLI encoding of these parameters is defined in [cli.md](cli.md).

Installing a version that is already installed at the correct path with a matching checksum is a no-op.

### update

Updates installed packages and manages pin status. See [cli.md](cli.md) for flags and examples.

With no arguments, updates all non-pinned packages to their latest versions in parallel. When packages are named explicitly, only those are updated — pinned packages are not skipped when named directly. A specific version can be targeted (including downgrading). Packages can be pinned or unpinned as part of the same operation.

A newer version is determined by the backend:
- `github`: a newer release tag exists
- `shasumurl`: the checksum file content has changed
- `kubeurl`: `stable.txt` points to a different version

### status

Reports whether a newer version is available for each installed package, without making any changes. Output includes the current installed version and the available version for any packages that have updates. See [cli.md](cli.md) for output format.

### list

Displays all installed packages with their name, current version, backend type, and local install path. See [cli.md](cli.md) for output format.

### uninstall

Removes the installed binary and its manifest. If a package installed multiple files (e.g., from an archive), all installed files are removed. See [cli.md](cli.md) for usage.

## Manifest

Each installed package has a manifest file in `~/.local/share/binmgr/`. The manifest records the backend type, source URL, current version, pin status, and the full set of install specs — each carrying its asset glob, traversal globs, checksum configuration, the resolved download URL, and the checksums and local paths of every installed file. All glob patterns are stored with `${VERSION}`/`${TAG}` placeholders intact so that updates can substitute the new version.

Manifest files are plain JSON and can be inspected directly. The full schema and worked examples for each supported pattern are in [datamodel.md](datamodel.md).

## Architecture

```
cmd/           CLI parsing, user-facing output, wiring
pkg/backend/   Backend interface and implementations (github, shasumurl, kubeurl)
pkg/manager/   Orchestration: install/update/status/list/uninstall lifecycle
pkg/manifest/  Manifest schema, storage, and loading
pkg/fetch/     HTTP downloading with progress reporting
pkg/extract/   Archive decompression and file extraction
pkg/verify/    Checksum computation and verification
```

The manager is the sole orchestrator: it calls the backend to resolve a version and asset list, then sequences fetch → verify → extract → verify → write to produce the final installed files and manifest. No layer calls another directly. All downloaded and extracted content is held in memory until the final file write. Full interface definitions and data flow are in [architecture.md](architecture.md).

### Key constraints

- Backends are stateless adapters. They resolve metadata (version, asset URLs) and do not touch the filesystem or the manifest.
- Manifests are owned exclusively by the manager layer. Backends return structured data; the manager decides what to persist.
- HTTP clients, filesystem paths, and output writers are injected rather than hard-coded, keeping every layer independently testable.

## Design Principles

1. **Backends are stateless adapters.** They talk to the network and return structured data. They do not touch the filesystem or the manifest.

2. **One place for dispatch.** URL-to-backend routing and type-string-to-backend routing live in a single registry.

3. **Fail loudly on type mismatches.** Unknown backend types, missing manifest fields, and unrecognized checksum formats are errors, not silent no-ops.

4. **Idempotent installs.** Installing the same version twice has no effect and produces no error.

5. **Checksum verification is not optional.** A backend that cannot provide a checksum must declare `none` explicitly. The default is to verify.
