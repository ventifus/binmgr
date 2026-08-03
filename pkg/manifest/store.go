package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"log/slog"
)

// ErrNotFound is returned by Load and Delete when no manifest exists for the requested ID.
var ErrNotFound = errors.New("package not found")

// LibDir returns the directory where manifest files are stored.
// It uses os.UserHomeDir() and falls back to os.Getenv("HOME") with a warning
// if the home directory cannot be determined.
func LibDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Warn("could not determine home directory; falling back to $HOME", "err", err)
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".local/share/binmgr/")
}

// effectiveDir returns dir if non-empty, otherwise falls back to LibDir().
func effectiveDir(dir string) string {
	if dir == "" {
		return LibDir()
	}
	return dir
}

// IDToFilename converts a package ID to a safe, collision-free filename.
// '/' becomes %2F and ':' becomes %3A.
func IDToFilename(id string) string {
	return strings.NewReplacer("/", "%2F", ":", "%3A").Replace(id)
}

// idToFilenameOld is the pre-migration encoding (both '/' and ':' → '_').
// Used only to locate and rename files written by older versions.
func idToFilenameOld(id string) string {
	return strings.NewReplacer("/", "_", ":", "_").Replace(id)
}

// Save writes a Package to disk as indented JSON.
// If dir is empty, LibDir() is used.
func Save(pkg *Package, dir string) error {
	d := effectiveDir(dir)
	if err := os.MkdirAll(d, 0700); err != nil {
		return fmt.Errorf("create manifest directory: %w", err)
	}

	data, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal package %s: %w", pkg.ID, err)
	}

	path := filepath.Join(d, IDToFilename(pkg.ID))
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write manifest for %s: %w", pkg.ID, err)
	}

	return nil
}

// Load reads and unmarshals a Package by its ID.
// If dir is empty, LibDir() is used.
// If the new-encoding filename is not found, falls back to the old encoding
// and renames the file as part of the migration.
func Load(id string, dir string) (*Package, error) {
	d := effectiveDir(dir)
	path := filepath.Join(d, IDToFilename(id))
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		oldPath := filepath.Join(d, idToFilenameOld(id))
		if oldData, err2 := os.ReadFile(oldPath); err2 == nil {
			if renameErr := os.Rename(oldPath, path); renameErr != nil {
				slog.Warn("failed to migrate manifest filename", "file", oldPath, "err", renameErr)
			}
			data, err = oldData, nil
		}
	}
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("read manifest for %s: %w", id, err)
	}

	var pkg Package
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("parse manifest for %s: %w", id, err)
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
		return nil, fmt.Errorf("read manifest directory: %w", err)
	}

	packages := make([]*Package, 0, len(entries))
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}

		path := filepath.Join(d, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("failed to read manifest file", "file", path, "err", err)
			continue
		}

		var pkg Package
		if err := json.Unmarshal(data, &pkg); err != nil {
			slog.Warn("failed to parse manifest file", "file", path, "err", err)
			continue
		}

		if newName := IDToFilename(pkg.ID); entry.Name() != newName {
			newPath := filepath.Join(d, newName)
			if err := os.Rename(path, newPath); err != nil {
				slog.Warn("failed to migrate manifest filename", "file", path, "err", err)
			}
		}

		packages = append(packages, &pkg)
	}

	return packages, nil
}

// Delete removes the manifest file for the given ID.
// If dir is empty, LibDir() is used.
// Falls back to the old filename encoding for packages not yet migrated.
func Delete(id string, dir string) error {
	d := effectiveDir(dir)
	path := filepath.Join(d, IDToFilename(id))
	if err := os.Remove(path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("delete manifest for %s: %w", id, err)
		}
		oldPath := filepath.Join(d, idToFilenameOld(id))
		if err2 := os.Remove(oldPath); err2 != nil {
			if errors.Is(err2, os.ErrNotExist) {
				return fmt.Errorf("%w: %s", ErrNotFound, id)
			}
			return fmt.Errorf("delete manifest for %s: %w", id, err2)
		}
	}
	return nil
}
