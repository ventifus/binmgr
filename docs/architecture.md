# binmgr Architecture

Internal design contracts for the Go implementation. See [spec.md](spec.md) for behavioral requirements and [datamodel.md](datamodel.md) for the manifest schema.

## Package Structure

```
cmd/           CLI parsing, user-facing output, wiring
pkg/backend/   Backend interface and implementations (github, shasumurl, kubeurl)
pkg/manager/   Orchestration: install/update/status/list/uninstall lifecycle
pkg/manifest/  Manifest schema, storage, and loading
pkg/fetch/     HTTP downloading with progress reporting
pkg/extract/   Archive decompression and file extraction
pkg/verify/    Checksum computation and verification
```

## Data Flow

The manager is the sole orchestrator. No layer calls another directly; all cross-layer coordination goes through the manager.

**Install:**
1. Dispatch source URL to backend via the registry
2. `backend.Resolve()` → version string and full asset list
3. For each unique asset URL referenced by the install specs:
   - `fetcher.Fetch(assetURL)` → `[]byte`
   - If the checksum strategy requires a separate file: `fetcher.Fetch(checksumURL)` → parse → expected digest map
   - `verifier.Verify(data, expected)`
4. For each install spec:
   - If traversal globs are present: `extractor.Extract(assetName, data, globs)` → `[]ExtractedFile`
   - Otherwise: treat the downloaded bytes as a single file
   - For each resulting file: `verifier.Compute(data, algorithms)` → record checksums
   - Write bytes to final path on disk
5. Build and write the manifest

**Update/Status:**
1. Load manifest from disk
2. `backend.Check(manifest)` → latest `Resolution`
3. Compare `Resolution.Version` to `manifest.Version`
4. If different (update): re-run the install flow using the existing specs from the manifest

---

## Backend Interface

```go
type Backend interface {
    // Resolve determines the available version and asset list for a source URL.
    Resolve(ctx context.Context, sourceURL *url.URL, opts ResolveOptions) (*Resolution, error)

    // Check returns the latest available version without installing.
    Check(ctx context.Context, pkg *manifest.Package) (*Resolution, error)

    // Type returns the string identifier stored in manifests ("github", "shasumurl", "kubeurl").
    Type() string

    // CanHandle reports whether this backend handles the given URL.
    CanHandle(u *url.URL) bool
}

type ResolveOptions struct {
    Version string // if non-empty, resolve this specific release; empty = latest
}

// Resolution is the result of Resolve or Check.
type Resolution struct {
    Version string  // github: tag; kubeurl: stable.txt content; shasumurl: SHA-256 of checksum file
    Assets  []Asset // all downloadable files for this version
}

// Asset is one downloadable file in a Resolution.
type Asset struct {
    Name      string            // filename, matched against asset_glob patterns
    URL       string            // fully resolved download URL
    Checksums map[string]string // pre-computed digests; non-empty for shasumurl only
}
```

### Backend Registry

A central registry maps URLs and type strings to `Backend` implementations. The `cmd` layer passes the source URL and optional `--type` override to the registry, which returns the appropriate backend. Adding a new backend means registering it; no other code changes.

---

## Manager Interface

```go
type Manager interface {
    Install(ctx context.Context, opts InstallOptions) error
    Update(ctx context.Context, opts UpdateOptions) ([]*UpdateResult, error)
    Status(ctx context.Context, packages []string) ([]*StatusResult, error)
    List(ctx context.Context) ([]*manifest.Package, error)
    Uninstall(ctx context.Context, packages []string) error
}

type InstallOptions struct {
    SourceURL   string
    Version     string // from @VERSION suffix; empty = latest
    Specs       []SpecOpts
    DefaultDir  string // default install directory; empty = ~/.local/bin/
    BackendType string // --type override; empty = auto-detect
    Pin         bool
}

type SpecOpts struct {
    AssetGlob      string
    TraversalGlobs []string
    LocalName      string
    Checksum       ChecksumOpts
}

type ChecksumOpts struct {
    Strategy      string // "auto" | "none" | "shared-file" | "per-asset" | "multisum" | "embedded"
    FileGlob      string // shared-file: glob to locate checksum file; multisum: data file glob
    OrderGlob     string // multisum: algorithm ordering file glob
    Suffix        string // per-asset: suffix appended to asset name
    TraversalGlob string // embedded: traversal glob inside archive
}

type UpdateOptions struct {
    Packages []PackageTarget // empty = all non-pinned packages
    Pin      bool
    Unpin    bool
}

type PackageTarget struct {
    ID      string
    Version string // empty = latest
}

type UpdateResult struct {
    ID         string
    OldVersion string
    NewVersion string
    Updated    bool
}

type StatusResult struct {
    ID               string
    InstalledVersion string
    LatestVersion    string
    Pinned           bool
    UpdateAvailable  bool
}
```

---

## Fetcher Interface

Downloads a URL into memory. Progress is reported to the user during the download.

```go
type Fetcher interface {
    Fetch(ctx context.Context, url string) ([]byte, error)
}
```

---

## Extractor Interface

Decompresses and extracts files from an in-memory archive. Format is detected from `name` (the original asset filename). For a plain compressed file (`.gz`, `.bz2` with no inner tar), `globs` are ignored and the single decompressed file is returned with an empty `SourcePath`.

```go
type Extractor interface {
    Extract(ctx context.Context, name string, data []byte, globs []string) ([]ExtractedFile, error)
}

type ExtractedFile struct {
    SourcePath string // path within archive; empty for direct binary downloads
    Data       []byte // file content
}
```

---

## Verifier Interface

Lives in `pkg/verify/`. Two operations: verify a download against expected checksums, and compute checksums for recording after extraction.

```go
type Verifier interface {
    // Verify checks data against all digests in expected. Returns an error
    // identifying which algorithms failed and what was found vs. expected.
    Verify(ctx context.Context, data []byte, expected map[string]string) error

    // Compute calculates digests for data using the given algorithms.
    // Used to record InstalledFile checksums after extraction.
    Compute(ctx context.Context, data []byte, algorithms []string) (map[string]string, error)
}
```
