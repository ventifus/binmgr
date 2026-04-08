package manifest

type Package struct {
	ID        string        `json:"id"`
	Backend   string        `json:"backend"`
	SourceURL string        `json:"source_url"`
	Version   string        `json:"version"`
	Pinned    bool          `json:"pinned,omitempty"`
	Specs     []InstallSpec `json:"specs"`
}

type InstallSpec struct {
	AssetGlob      string           `json:"asset_glob"`
	TraversalGlobs []string         `json:"traversal_globs,omitempty"`
	LocalName      string           `json:"local_name,omitempty"`
	Checksum       ChecksumConfig   `json:"checksum"`
	Asset          *DownloadedAsset `json:"asset,omitempty"`
	InstalledFiles []InstalledFile  `json:"installed_files,omitempty"`
}

type ChecksumConfig struct {
	Strategy      string `json:"strategy"`
	FileGlob      string `json:"file_glob,omitempty"`
	Suffix        string `json:"suffix,omitempty"`
	DataGlob      string `json:"data_glob,omitempty"`
	OrderGlob     string `json:"order_glob,omitempty"`
	TraversalGlob string `json:"traversal_glob,omitempty"`
}

type DownloadedAsset struct {
	URL               string            `json:"url"`
	Checksums         map[string]string `json:"checksums"`
	ChecksumSourceURL string            `json:"checksum_source_url,omitempty"`
}

type InstalledFile struct {
	SourcePath string            `json:"source_path,omitempty"`
	LocalPath  string            `json:"local_path"`
	Checksums  map[string]string `json:"checksums"`
}
