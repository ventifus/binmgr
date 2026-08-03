package manifest

// Package is the top-level manifest record for an installed package.
type Package struct {
	ID        string        `json:"id"`
	Backend   string        `json:"backend"`
	SourceURL string        `json:"source_url"`
	Version   string        `json:"version"`
	Pinned    bool          `json:"pinned,omitempty"`
	Specs     []InstallSpec `json:"specs"`
}

// InstallSpec records how one set of files was installed (asset selection, checksum, installed paths).
type InstallSpec struct {
	AssetGlob      string           `json:"asset_glob"`
	TraversalGlobs []string         `json:"traversal_globs,omitempty"`
	LocalName      string           `json:"local_name,omitempty"`
	Checksum       ChecksumConfig   `json:"checksum"`
	Asset          *DownloadedAsset `json:"asset,omitempty"`
	InstalledFiles []InstalledFile  `json:"installed_files,omitempty"`
}

// ChecksumConfig is the checksum strategy and arguments recorded at install time.
type ChecksumConfig struct {
	Strategy      string `json:"strategy"`
	FileGlob      string `json:"file_glob,omitempty"`
	Suffix        string `json:"suffix,omitempty"`
	DataGlob      string `json:"data_glob,omitempty"`
	OrderGlob     string `json:"order_glob,omitempty"`
	TraversalGlob string `json:"traversal_glob,omitempty"`
}

// DownloadedAsset records the URL and checksums of the downloaded asset.
type DownloadedAsset struct {
	URL               string            `json:"url"`
	Checksums         map[string]string `json:"checksums"`
	ChecksumSourceURL string            `json:"checksum_source_url,omitempty"`
}

// InstalledFile records the path and checksums of one installed file on disk.
type InstalledFile struct {
	SourcePath string            `json:"source_path,omitempty"`
	LocalPath  string            `json:"local_path"`
	Checksums  map[string]string `json:"checksums"`
}
