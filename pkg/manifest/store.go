package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/apex/log"
)

// LibDir returns the directory where manifest files are stored.
func LibDir() string {
	return filepath.Join(os.Getenv("HOME"), ".local/share/binmgr/")
}

// effectiveDir returns dir if non-empty, otherwise falls back to LibDir().
func effectiveDir(dir string) string {
	if dir == "" {
		return LibDir()
	}
	return dir
}

// IDToFilename converts a package ID to a safe filename by replacing
// '/' and ':' with '_'.
func IDToFilename(id string) string {
	result := make([]byte, len(id))
	for i := range len(id) {
		switch id[i] {
		case '/', ':':
			result[i] = '_'
		default:
			result[i] = id[i]
		}
	}
	return string(result)
}

// Save writes a Package to disk as indented JSON.
// If dir is empty, LibDir() is used.
func Save(pkg *Package, dir string) error {
	d := effectiveDir(dir)
	if err := os.MkdirAll(d, 0700); err != nil {
		return fmt.Errorf("failed to create manifest directory: %w", err)
	}

	data, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal package %s: %w", pkg.ID, err)
	}

	path := filepath.Join(d, IDToFilename(pkg.ID))
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write manifest for %s: %w", pkg.ID, err)
	}

	return nil
}

// Load reads and unmarshals a Package by its ID.
// If dir is empty, LibDir() is used.
func Load(id string, dir string) (*Package, error) {
	path := filepath.Join(effectiveDir(dir), IDToFilename(id))
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("package not found: %s", id)
		}
		return nil, fmt.Errorf("failed to read manifest for %s: %w", id, err)
	}

	var pkg Package
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("failed to parse manifest for %s: %w", id, err)
	}

	return &pkg, nil
}

// LoadAll reads all Package manifests from dir. Files that fail to parse
// are skipped with a warning logged. Returns nil (not an error) if the
// directory does not yet exist. If dir is empty, LibDir() is used.
func LoadAll(dir string) ([]*Package, error) {
	d := effectiveDir(dir)

	entries, err := os.ReadDir(d)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read manifest directory: %w", err)
	}

	packages := make([]*Package, 0, len(entries))
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}

		path := filepath.Join(d, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			log.WithField("file", path).WithError(err).Warn("failed to read manifest file")
			continue
		}

		var pkg Package
		if err := json.Unmarshal(data, &pkg); err != nil {
			log.WithField("file", path).WithError(err).Warn("failed to parse manifest file")
			continue
		}

		packages = append(packages, &pkg)
	}

	return packages, nil
}

// Delete removes the manifest file for the given ID.
// If dir is empty, LibDir() is used.
func Delete(id string, dir string) error {
	path := filepath.Join(effectiveDir(dir), IDToFilename(id))
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("package not found: %s", id)
		}
		return fmt.Errorf("failed to delete manifest for %s: %w", id, err)
	}
	return nil
}
