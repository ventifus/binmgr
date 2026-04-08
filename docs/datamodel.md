# binmgr Data Model

Each installed package is represented by a manifest file in `~/.local/share/binmgr/`. Manifests are plain JSON. This document defines the schema.

See [spec.md](spec.md) for the concepts these fields implement.

## Schema

### Package

The top-level record for an installed package.

```
Package
├── id          string        Canonical package identifier (see Manifest Files)
├── backend     string        "github" | "shasumurl" | "kubeurl"
├── source_url  string        URL used to install; used by update/status to check for newer versions
├── version     string        Currently installed version string (tag, content hash, or stable pointer value)
├── pinned      bool          If true, update skips this package
└── specs       []InstallSpec One entry per declared install spec
```

### InstallSpec

One spec per file or set of files the user declared to install. A package with multiple specs (e.g., a binary and a shared library from the same archive) has multiple entries here.

```
InstallSpec
├── asset_glob      string          Pattern to select the file to download (unexpanded)
├── traversal_globs []string        Patterns to navigate into archives, one per level (unexpanded)
├── local_name      string          Override for the installed filename or absolute path; empty means use basename
├── checksum        ChecksumConfig  How to verify the download
├── asset           DownloadedAsset The file that was fetched (absent until first install)
└── installed_files []InstalledFile Files on disk produced by this spec (absent until first install)
```

`asset_glob`, `traversal_globs`, and any glob in `checksum` are stored with `${VERSION}` and `${TAG}` placeholders intact. They are expanded at runtime using the resolved version.

### ChecksumConfig

```
ChecksumConfig
├── strategy    string   "shared-file" | "per-asset" | "multisum" | "embedded" | "none"
│
│   strategy = "shared-file"
├── file_glob   string   Glob to locate the checksum file among release assets (unexpanded)
│
│   strategy = "per-asset"
├── suffix      string   Suffix appended to the asset name to find its checksum file (e.g. ".sha256")
│
│   strategy = "multisum"
├── data_glob   string   Glob to locate the checksums data file (unexpanded)
└── order_glob  string   Glob to locate the algorithm-ordering file (unexpanded)
│
│   strategy = "embedded"
└── traversal_glob  string  Traversal glob to locate the checksum file inside the archive (unexpanded)
│
│   strategy = "none"
│   (no additional fields)
```

Fields not relevant to the chosen strategy are omitted.

### DownloadedAsset

The file that was actually fetched from the source. Populated after the first install; updated on each upgrade.

```
DownloadedAsset
├── url                 string              The resolved download URL (fully expanded, no placeholders)
├── checksums           map[string]string   Algorithm → hex digest, used to verify the download
└── checksum_source_url string              URL the checksum was fetched from (for audit); empty for per-asset and embedded
```

### InstalledFile

One entry per file written to disk by this spec. For a direct binary download there is one entry. For an archive, there is one entry per extracted file that matched the traversal globs.

```
InstalledFile
├── source_path  string              Path of this file within the archive; empty for direct binary downloads
├── local_path   string              Absolute path on disk
└── checksums    map[string]string   Algorithm → hex digest, computed after extraction for future re-verification
```

## Manifest Files

Manifests are stored in `~/.local/share/binmgr/`. The filename is the package `id` with `/` and `:` replaced by `_`.

The `id` is the canonical form of the source URL:

| Backend | Source URL example | ID |
|---------|-------------------|----|
| `github` | `https://github.com/casey/just` | `github.com/casey/just` |
| `shasumurl` | `https://mirror.openshift.com/.../sha256sum.txt` | `mirror.openshift.com/.../sha256sum.txt` |
| `kubeurl` | source `dl.k8s.io/release/stable.txt`, `--file bin/linux/amd64/kubectl` | `dl.k8s.io/bin/linux/amd64/kubectl` |

For `kubeurl`, the `id` is `{hostname}/{asset_glob}` — the version-pointer URL's hostname joined with the asset path from `--file`. This makes each binary a distinct package even when multiple binaries share the same version pointer.

## Examples

### E1 — Direct binary with rename (knative/func)

```json
{
    "id": "github.com/knative/func",
    "backend": "github",
    "source_url": "https://github.com/knative/func",
    "version": "knative-v1.19.3",
    "pinned": false,
    "specs": [
        {
            "asset_glob": "func_linux_amd64",
            "local_name": "kn-func",
            "checksum": { "strategy": "none" },
            "asset": {
                "url": "https://github.com/knative/func/releases/download/knative-v1.19.3/func_linux_amd64",
                "checksums": { "sha-256": "ca05a7b9..." }
            },
            "installed_files": [
                {
                    "local_path": "/home/user/.local/bin/kn-func",
                    "checksums": { "sha-256": "ca05a7b9..." }
                }
            ]
        }
    ]
}
```

Note: no `traversal_globs` (direct binary); `local_name` overrides the asset name.

### E3 — Tar.gz with basename traversal, static checksum filename (casey/just)

```json
{
    "id": "github.com/casey/just",
    "backend": "github",
    "source_url": "https://github.com/casey/just",
    "version": "1.46.0",
    "specs": [
        {
            "asset_glob": "just-${VERSION}-x86_64-unknown-linux-musl.tar.gz",
            "traversal_globs": ["just"],
            "checksum": {
                "strategy": "shared-file",
                "file_glob": "SHA256SUMS"
            },
            "asset": {
                "url": "https://github.com/casey/just/releases/download/1.46.0/just-1.46.0-x86_64-unknown-linux-musl.tar.gz",
                "checksums": { "sha-256": "79966e6e..." },
                "checksum_source_url": "https://github.com/casey/just/releases/download/1.46.0/SHA256SUMS"
            },
            "installed_files": [
                {
                    "source_path": "just",
                    "local_path": "/home/user/.local/bin/just",
                    "checksums": { "sha-256": "..." }
                }
            ]
        }
    ]
}
```

Note: `asset_glob` and `traversal_globs` stored with `${VERSION}` unexpanded. `local_name` absent — basename of `source_path` (`just`) is used.

### E7 — Multiple files from one archive to different directories (git-ecosystem/git-credential-manager)

```json
{
    "id": "github.com/git-ecosystem/git-credential-manager",
    "backend": "github",
    "source_url": "https://github.com/git-ecosystem/git-credential-manager",
    "version": "v2.6.1",
    "specs": [
        {
            "asset_glob": "gcm-linux_amd64.*.tar.gz",
            "traversal_globs": ["git-credential-manager"],
            "checksum": { "strategy": "none" },
            "asset": {
                "url": "https://github.com/git-ecosystem/git-credential-manager/releases/download/v2.6.1/gcm-linux_amd64.2.6.1.tar.gz",
                "checksums": {}
            },
            "installed_files": [
                {
                    "source_path": "git-credential-manager",
                    "local_path": "/home/user/.local/bin/git-credential-manager",
                    "checksums": {}
                }
            ]
        },
        {
            "asset_glob": "gcm-linux_amd64.*.tar.gz",
            "traversal_globs": ["libHarfBuzzSharp.so"],
            "local_name": "/home/user/.local/lib/libHarfBuzzSharp.so",
            "checksum": { "strategy": "none" },
            "asset": {
                "url": "https://github.com/git-ecosystem/git-credential-manager/releases/download/v2.6.1/gcm-linux_amd64.2.6.1.tar.gz",
                "checksums": {}
            },
            "installed_files": [
                {
                    "source_path": "libHarfBuzzSharp.so",
                    "local_path": "/home/user/.local/lib/libHarfBuzzSharp.so",
                    "checksums": {}
                }
            ]
        }
    ]
}
```

Note: two specs share the same `asset_glob` — the archive is downloaded once. The second spec uses an absolute path as `local_name` to override the install directory.

### E10 — Multisum (mikefarah/yq)

```json
{
    "id": "github.com/mikefarah/yq",
    "backend": "github",
    "source_url": "https://github.com/mikefarah/yq",
    "version": "v4.50.1",
    "specs": [
        {
            "asset_glob": "yq_linux_amd64.tar.gz",
            "traversal_globs": ["./yq_linux_amd64"],
            "local_name": "yq",
            "checksum": {
                "strategy": "multisum",
                "data_glob": "checksums",
                "order_glob": "checksums_hashes_order"
            },
            "asset": {
                "url": "https://github.com/mikefarah/yq/releases/download/v4.50.1/yq_linux_amd64.tar.gz",
                "checksums": {
                    "sha-256": "00534770...",
                    "sha-512": "4e40291f..."
                },
                "checksum_source_url": "https://github.com/mikefarah/yq/releases/download/v4.50.1/checksums"
            },
            "installed_files": [
                {
                    "source_path": "./yq_linux_amd64",
                    "local_path": "/home/user/.local/bin/yq",
                    "checksums": { "sha-256": "..." }
                }
            ]
        }
    ]
}
```

### E11 — Kubernetes version pointer (kubectl)

```json
{
    "id": "dl.k8s.io/bin/linux/amd64/kubectl",
    "backend": "kubeurl",
    "source_url": "https://dl.k8s.io/release/stable.txt",
    "version": "v1.35.0",
    "specs": [
        {
            "asset_glob": "bin/linux/amd64/kubectl",
            "checksum": {
                "strategy": "per-asset",
                "suffix": ".sha256"
            },
            "asset": {
                "url": "https://dl.k8s.io/v1.35.0/bin/linux/amd64/kubectl",
                "checksums": { "sha-256": "a2e984a1..." },
                "checksum_source_url": "https://dl.k8s.io/v1.35.0/bin/linux/amd64/kubectl.sha256"
            },
            "installed_files": [
                {
                    "local_path": "/home/user/.local/bin/kubectl",
                    "checksums": { "sha-256": "a2e984a1..." }
                }
            ]
        }
    ]
}
```

Note: `asset_glob` is a path suffix; the kubeurl backend prepends the base URL and resolved version. No `traversal_globs` — the download is a bare binary.

### E12 — OpenShift mirror (oc)

```json
{
    "id": "mirror.openshift.com/pub/openshift-v4/x86_64/clients/ocp/stable/sha256sum.txt",
    "backend": "shasumurl",
    "source_url": "https://mirror.openshift.com/pub/openshift-v4/x86_64/clients/ocp/stable/sha256sum.txt",
    "version": "a3f7b2c1d4e8f9ab12cd34ef56789012abcdef1234567890abcdef1234567890",
    "specs": [
        {
            "asset_glob": "openshift-client-linux-amd64-rhel9-*",
            "traversal_globs": ["oc"],
            "checksum": { "strategy": "none" },
            "asset": {
                "url": "https://mirror.openshift.com/pub/openshift-v4/x86_64/clients/ocp/stable/openshift-client-linux-amd64-rhel9-4.20.10.tar.gz",
                "checksums": { "sha-256": "d4e8f7a2b1c3..." },
                "checksum_source_url": "https://mirror.openshift.com/pub/openshift-v4/x86_64/clients/ocp/stable/sha256sum.txt"
            },
            "installed_files": [
                {
                    "source_path": "oc",
                    "local_path": "/home/user/.local/bin/oc",
                    "checksums": { "sha-256": "9f1e2d3c..." }
                }
            ]
        }
    ]
}
```

Note: `version` is the SHA-256 hex digest of the sha256sum.txt file's full content — not a release tag. Update detection compares this hash on each `status`/`update` run. `checksum_source_url` points to the sha256sum.txt that provided the asset checksums. No `${VERSION}` in globs since the shasumurl backend uses content comparison for update detection, not version substitution. `InstallSpec.checksum.strategy` is `"none"` because no user-specified additional checksum fetch is needed; checksums are always supplied by the backend from the parsed index.
