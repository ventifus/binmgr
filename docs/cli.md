# binmgr CLI Design

## Conventions

- The `https://` scheme is optional on all URLs; binmgr adds it automatically.
- `${VERSION}` and `${TAG}` in any glob or pattern are substituted with the resolved version at runtime. See [Variable Substitution](spec.md#variable-substitution).
- Package identity is the canonical ID from the manifest (e.g. `github.com/casey/just`). Commands that accept package names use this form.
- All commands support `--help`.

---

## Global Flags

```
--loglevel LEVEL    Log verbosity: debug, info, warn, error (default: warn)
```

---

## install

Download and install one package. Records a manifest for future `update` and `status`.

```
binmgr install URL[@VERSION] [flags]
```

`@VERSION` pins the install to a specific release tag (e.g. `github.com/casey/just@1.40.0`). When combined with `--pin`, the package is also excluded from future `update` runs. Omit `@VERSION` to install the latest release.

### Flags

```
-f, --file SPEC         Install spec: what to download, extract, and name (repeatable; see below)
    --checksum STRATEGY Checksum strategy (see below; default: auto)
    --dir PATH          Default install directory (default: ~/.local/bin/)
    --type TYPE         Backend override: github | shasumurl | kubeurl
                        Auto-detected for github.com and dl.k8s.io URLs
    --pin               Pin this package to whatever version is installed.
```

### The `--file` Spec

Each `--file` flag declares one install spec. Specify `--file` multiple times to install multiple files under one manifest.

**Format:** `ASSET_GLOB[!TRAVERSAL_GLOB...][@LOCAL_NAME]`

- `ASSET_GLOB` — glob matched against the available assets at the source.
- `!TRAVERSAL_GLOB` — descends one level into an archive. Repeatable for nested archives. Matched against both full entry path and basename.
- `@LOCAL_NAME` — the installed filename or absolute path. Omit to use the basename of the last matched path. If an absolute path, also overrides `--dir` for this file.

All glob segments support `${VERSION}` and `${TAG}` substitution. They are stored unexpanded in the manifest.

At least one `--file` is required for the `github` and `shasumurl` backends, where the URL identifies a release but not a specific asset. For `kubeurl`, `--file` specifies the path suffix used to construct the download URL and is also required.

### The `--checksum` Strategy

```
--checksum auto                        Try to detect a shared checksum file; error if none found (default)
--checksum none                        Skip verification; suppress the no-checksum warning
--checksum shared-file:GLOB            Shared file listing all asset checksums; GLOB locates it
--checksum per-asset:SUFFIX            Per-asset file; SUFFIX appended to asset name (e.g. .sha256)
--checksum multisum[:DATA_GLOB[:ORDER_GLOB]]
                                       HashiCorp multisum format; defaults: "checksums" and
                                       "checksums_hashes_order"
--checksum embedded:TRAVERSAL_GLOB     Checksum file inside the archive, found by traversal glob
```


`GLOB` in `shared-file` and `multisum` supports `${VERSION}` and `${TAG}` substitution.

### Examples

**Direct binary that needs renaming** (E1 — https://github.com/knative/func):
```sh
binmgr install github.com/knative/func \
  --file "func_linux_amd64@kn-func"
```

**Tar.gz, basename traversal, static checksum filename** (E3 — https://github.com/casey/just):
```sh
binmgr install github.com/casey/just \
  --file "just-${VERSION}-x86_64-unknown-linux-musl.tar.gz!just" \
  --checksum "shared-file:SHA256SUMS"
```

**Tar.gz, versioned checksum filename** (E9 — https://github.com/aquasecurity/trivy):
```sh
binmgr install github.com/aquasecurity/trivy \
  --file "*Linux-64bit.tar.gz!trivy" \
  --checksum "shared-file:trivy_${VERSION}_checksums.txt"
```

**Tar.gz, versioned path traversal, versioned checksum filename** (E4 — https://github.com/cli/cli):
```sh
binmgr install github.com/cli/cli \
  --file "gh_${VERSION}_linux_amd64.tar.gz!gh_${VERSION}_linux_amd64/bin/gh" \
  --checksum "shared-file:gh_${VERSION}_checksums.txt"
```

**Zip with path traversal, per-asset checksum** (E5 — https://github.com/Azure/kubelogin):
```sh
binmgr install github.com/Azure/kubelogin \
  --file "kubelogin-linux-amd64.zip!bin/linux_amd64/kubelogin@kubelogin" \
  --checksum "per-asset:.sha256"
```

**Gzip-compressed single binary** (E2 — https://github.com/tree-sitter/tree-sitter):
```sh
binmgr install github.com/tree-sitter/tree-sitter \
  --file "tree-sitter-linux-x64.gz@tree-sitter" \
  --checksum none
```

**Bzip2 tarball** (E6 — https://github.com/aristocratos/btop):
```sh
binmgr install github.com/aristocratos/btop \
  --file "*x86_64-linux*.tbz!btop" \
  --checksum none
```

**Multiple files from one archive to different directories** (E7 — https://github.com/git-ecosystem/git-credential-manager):
```sh
binmgr install github.com/git-ecosystem/git-credential-manager \
  --file "gcm-linux_amd64.*.tar.gz!git-credential-manager" \
  --file "gcm-linux_amd64.*.tar.gz!libHarfBuzzSharp.so@~/.local/lib/libHarfBuzzSharp.so" \
  --checksum none
```
Both `--file` flags share the same asset glob; the archive is downloaded once.

**Multisum** (E10 — https://github.com/mikefarah/yq):
```sh
binmgr install github.com/mikefarah/yq \
  --file "yq_linux_amd64.tar.gz!./yq_linux_amd64@yq" \
  --checksum multisum
```

**Kubernetes** (E11):
```sh
binmgr install dl.k8s.io/release/stable.txt \
  --file bin/linux/amd64/kubectl \
  --checksum "per-asset:.sha256"
```
The `kubeurl` backend is auto-detected from the `dl.k8s.io` hostname. The `--file` value is the path suffix; the backend constructs the full download URL by combining the base URL with the resolved version.

**OpenShift mirror** (E12):
```sh
binmgr install --type shasumurl \
  mirror.openshift.com/pub/openshift-v4/x86_64/clients/ocp/stable/sha256sum.txt \
  --file "openshift-client-linux-amd64-rhel9-*!oc"
```
The checksum is inherent in the shasumurl format; no `--checksum` flag needed.

**Install a specific version and pin it**:
```sh
binmgr install github.com/casey/just@1.40.0 \
  --file "just-${VERSION}-x86_64-unknown-linux-musl.tar.gz!just" \
  --checksum "shared-file:SHA256SUMS" \
  --pin
```

**Install to a non-default directory**:
```sh
binmgr install github.com/casey/just \
  --file "just-${VERSION}-x86_64-unknown-linux-musl.tar.gz!just" \
  --checksum "shared-file:SHA256SUMS" \
  --dir /usr/local/bin
```

---

## update

Update installed packages and manage pin status.

```
binmgr update [PACKAGE[@VERSION]...] [flags]
```

With no arguments, updates all non-pinned packages to their latest versions. Updates run in parallel.

When one or more packages are named explicitly, only those packages are updated — pinned packages are not skipped when named directly. A specific version can be appended to a package name with `@VERSION` to target a particular release (including downgrading).

### Flags

```
    --pin     Pin each named package at the version it is updated to
    --unpin   Remove the pin from each named package, then update to latest
```

`--pin` and `--unpin` require at least one package to be named. They cannot be combined.

### Examples

```sh
# Update all non-pinned packages
binmgr update

# Update one package to latest
binmgr update github.com/casey/just

# Update to a specific version
binmgr update github.com/casey/just@1.40.0

# Update to latest and pin there
binmgr update github.com/casey/just --pin

# Downgrade to a specific version and pin there
binmgr update github.com/casey/just@1.40.0 --pin

# Remove pin and update to latest
binmgr update github.com/casey/just --unpin

# Update several packages and pin all of them
binmgr update github.com/casey/just github.com/aquasecurity/trivy --pin
```

---

## status

Report whether newer versions are available, without making any changes.

```
binmgr status [PACKAGE...]
```

With no arguments, checks all installed packages. With package IDs, checks only those.

### Output

```
github.com/casey/just                         1.46.0           up to date
github.com/aquasecurity/trivy                 v0.67.0    →     v0.68.2
dl.k8s.io/bin/linux/amd64/kubectl             v1.34.0    →     v1.35.0
github.com/knative/func                       knative-v1.19.3  up to date  [pinned]
```

Exit code is 0 if all packages are up to date, 1 if any updates are available.

---

## list

Show all installed packages.

```
binmgr list
```

### Output

```
github.com/casey/just                         1.46.0
  ~/.local/bin/just

github.com/git-ecosystem/git-credential-manager  v2.6.1
  ~/.local/bin/git-credential-manager
  ~/.local/lib/libHarfBuzzSharp.so

dl.k8s.io/bin/linux/amd64/kubectl            v1.35.0
  ~/.local/bin/kubectl

github.com/knative/func                       knative-v1.19.3  [pinned]
  ~/.local/bin/kn-func
```

---

## uninstall

Remove an installed package: deletes all installed files and the manifest.

```
binmgr uninstall PACKAGE...
```

### Examples

```sh
binmgr uninstall github.com/casey/just

# Uninstall multiple
binmgr uninstall github.com/casey/just github.com/aquasecurity/trivy
```

Exits with an error if the package is not installed.

