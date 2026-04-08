package manager

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ventifus/binmgr/pkg/backend"
)

// parseSha256Sums parses the standard sha256sums format.
// Accepts both text mode ("{hex}  {filename}", two spaces) and binary mode
// ("{hex} *{filename}", space + asterisk — the asterisk is stripped from the
// filename). Returns map[filename]hex. Errors on malformed lines.
func parseSha256Sums(data []byte) (map[string]string, error) {
	result := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var hex, filename string
		if idx := strings.Index(line, "  "); idx >= 0 {
			// Text mode: "{hex}  {filename}"
			hex = line[:idx]
			filename = line[idx+2:]
		} else if idx := strings.Index(line, " *"); idx >= 0 {
			// Binary mode: "{hex} *{filename}"
			hex = line[:idx]
			filename = line[idx+2:]
		} else {
			return nil, fmt.Errorf("parseSha256Sums: malformed line (missing two-space or space-asterisk separator): %q", line)
		}
		result[filename] = hex
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// parseMultisum parses HashiCorp multisum format.
// orderFile has one algorithm name per line (in column order).
// dataFile has one asset per line with whitespace-separated hex columns.
// Find the row for targetName; build map[algorithm]hex using column-to-algorithm
// mapping from the order file.
// Algorithm names in the order file (e.g. "SHA256") map to "sha-256" / "sha-512" keys.
func parseMultisum(dataFile, orderFile []byte, targetName string) (map[string]string, error) {
	// Parse order file to get algorithm column order.
	var algorithms []string
	scanner := bufio.NewScanner(bytes.NewReader(orderFile))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		algorithms = append(algorithms, normalizeAlgorithm(line))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Parse data file to find the row for targetName.
	scanner = bufio.NewScanner(bytes.NewReader(dataFile))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		// First field is the asset name.
		name := fields[0]
		if name != targetName {
			continue
		}
		// Remaining fields are hex values in algorithm order.
		hexValues := fields[1:]
		result := make(map[string]string, len(algorithms))
		for i, alg := range algorithms {
			if i < len(hexValues) {
				result[alg] = hexValues[i]
			}
		}
		return result, nil
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return nil, fmt.Errorf("parseMultisum: target %q not found in data file", targetName)
}

// normalizeAlgorithm converts algorithm names from the order file (e.g. "SHA256", "SHA512")
// to canonical "sha-NNN" keys.
func normalizeAlgorithm(name string) string {
	switch strings.ToUpper(name) {
	case "SHA256":
		return "sha-256"
	case "SHA512":
		return "sha-512"
	case "SHA1":
		return "sha-1"
	case "MD5":
		return "md5"
	default:
		return strings.ToLower(name)
	}
}

// findAsset finds an asset in a Resolution by glob pattern.
// Uses filepath.Match. Returns error if no match.
func findAsset(resolution *backend.Resolution, glob string) (*backend.Asset, error) {
	for i := range resolution.Assets {
		matched, err := filepath.Match(glob, resolution.Assets[i].Name)
		if err != nil {
			return nil, fmt.Errorf("findAsset: invalid glob %q: %w", glob, err)
		}
		if matched {
			return &resolution.Assets[i], nil
		}
	}
	return nil, fmt.Errorf("findAsset: no asset matched glob %q", glob)
}

// resolveChecksums determines the expected checksums for an asset given the strategy.
// Returns nil for "none" (caller skips Verify).
// assetName is the filename of the downloaded asset.
// assetData is the downloaded asset bytes (used only for "embedded" strategy).
// assetURL is the full URL of the downloaded asset (needed for per-asset strategy).
// assetChecksums is the pre-populated checksum from the shasumurl backend (non-nil = use directly).
// resolution is the full Resolution from the backend.
// tag is the raw release tag for ${TAG}/${VERSION} expansion.
func (m *mgr) resolveChecksums(
	ctx context.Context,
	opts ChecksumOpts,
	assetName string,
	assetData []byte,
	assetURL string,
	assetChecksums map[string]string,
	resolution *backend.Resolution,
	tag string,
) (map[string]string, error) {
	// shasumurl shortcut: if the backend already provided checksums, use them directly.
	if len(assetChecksums) > 0 {
		return assetChecksums, nil
	}

	switch opts.Strategy {
	case "none":
		return nil, nil

	case "auto":
		return m.resolveChecksumsAuto(ctx, assetName, resolution)

	case "shared-file":
		return m.resolveChecksumsSharedFile(ctx, opts, assetName, resolution, tag)

	case "per-asset":
		return m.resolveChecksumsPerAsset(ctx, opts, assetURL)

	case "multisum":
		return m.resolveChecksumsMultisum(ctx, opts, assetName, resolution, tag)

	case "embedded":
		return m.resolveChecksumsEmbedded(ctx, opts, assetName, assetData, tag)

	default:
		return nil, fmt.Errorf("resolveChecksums: unknown strategy %q", opts.Strategy)
	}
}

// resolveChecksumsAuto implements the "auto" heuristic.
// Searches resolution.Assets names for known checksum file patterns.
func (m *mgr) resolveChecksumsAuto(ctx context.Context, assetName string, resolution *backend.Resolution) (map[string]string, error) {
	// Step 1: exact name matches in priority order.
	exactNames := []string{"SHA256SUMS", "SHA256SUMS.txt", "checksums.txt", "checksums.sha256"}
	for _, candidate := range exactNames {
		for i := range resolution.Assets {
			if resolution.Assets[i].Name == candidate {
				return m.fetchAndParseSha256Sums(ctx, resolution.Assets[i].URL, assetName)
			}
		}
	}

	// Step 2: wildcard matches.
	wildcards := []string{"*checksums*", "*sha256*"}
	for _, glob := range wildcards {
		for i := range resolution.Assets {
			matched, err := filepath.Match(glob, resolution.Assets[i].Name)
			if err != nil {
				continue
			}
			if matched {
				return m.fetchAndParseSha256Sums(ctx, resolution.Assets[i].URL, assetName)
			}
		}
	}

	return nil, fmt.Errorf("checksum file not found; tried: [SHA256SUMS, SHA256SUMS.txt, checksums.txt, checksums.sha256, *checksums*, *sha256*]; use --checksum none to skip or specify an explicit strategy")
}

// resolveChecksumsSharedFile implements the "shared-file" strategy.
func (m *mgr) resolveChecksumsSharedFile(ctx context.Context, opts ChecksumOpts, assetName string, resolution *backend.Resolution, tag string) (map[string]string, error) {
	expandedGlob := ExpandVars(opts.FileGlob, tag)
	asset, err := findAsset(resolution, expandedGlob)
	if err != nil {
		return nil, fmt.Errorf("shared-file: %w", err)
	}
	return m.fetchAndParseSha256Sums(ctx, asset.URL, assetName)
}

// fetchAndParseSha256Sums fetches a checksum file and looks up assetName in it.
func (m *mgr) fetchAndParseSha256Sums(ctx context.Context, url, assetName string) (map[string]string, error) {
	data, err := m.fetcher.Fetch(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetch checksum file %q: %w", url, err)
	}
	sums, err := parseSha256Sums(data)
	if err != nil {
		return nil, err
	}
	hex, ok := sums[assetName]
	if !ok {
		return nil, fmt.Errorf("asset %q not found in checksum file %q", assetName, url)
	}
	return map[string]string{"sha-256": hex}, nil
}

// resolveChecksumsPerAsset implements the "per-asset" strategy.
func (m *mgr) resolveChecksumsPerAsset(ctx context.Context, opts ChecksumOpts, assetURL string) (map[string]string, error) {
	checksumURL := assetURL + opts.Suffix
	data, err := m.fetcher.Fetch(ctx, checksumURL)
	if err != nil {
		return nil, fmt.Errorf("per-asset: fetch checksum file %q: %w", checksumURL, err)
	}
	// Derive assetName from the URL (everything after the last slash, before the suffix).
	assetName := assetURL[strings.LastIndex(assetURL, "/")+1:]
	content := strings.TrimSpace(string(data))
	var hexDigest string
	if strings.Contains(content, "  ") || strings.Contains(content, " *") {
		// sha256sums-format line: "<hex>  <filename>" or "<hex> *<filename>"
		parsed, err := parseSha256Sums(data)
		if err != nil {
			return nil, fmt.Errorf("per-asset: parsing checksum file: %w", err)
		}
		digest, ok := parsed[assetName]
		if !ok {
			return nil, fmt.Errorf("per-asset: asset %q not found in checksum file", assetName)
		}
		hexDigest = digest
	} else {
		hexDigest = content
	}
	return map[string]string{"sha-256": hexDigest}, nil
}

// resolveChecksumsMultisum implements the "multisum" strategy.
func (m *mgr) resolveChecksumsMultisum(ctx context.Context, opts ChecksumOpts, assetName string, resolution *backend.Resolution, tag string) (map[string]string, error) {
	expandedFileGlob := ExpandVars(opts.FileGlob, tag)
	expandedOrderGlob := ExpandVars(opts.OrderGlob, tag)

	dataAsset, err := findAsset(resolution, expandedFileGlob)
	if err != nil {
		return nil, fmt.Errorf("multisum data file: %w", err)
	}
	orderAsset, err := findAsset(resolution, expandedOrderGlob)
	if err != nil {
		return nil, fmt.Errorf("multisum order file: %w", err)
	}

	dataFile, err := m.fetcher.Fetch(ctx, dataAsset.URL)
	if err != nil {
		return nil, fmt.Errorf("multisum: fetch data file %q: %w", dataAsset.URL, err)
	}
	orderFile, err := m.fetcher.Fetch(ctx, orderAsset.URL)
	if err != nil {
		return nil, fmt.Errorf("multisum: fetch order file %q: %w", orderAsset.URL, err)
	}

	return parseMultisum(dataFile, orderFile, assetName)
}

// resolveChecksumsEmbedded implements the "embedded" strategy.
func (m *mgr) resolveChecksumsEmbedded(ctx context.Context, opts ChecksumOpts, assetName string, assetData []byte, tag string) (map[string]string, error) {
	expandedGlob := ExpandVars(opts.TraversalGlob, tag)
	files, err := m.extractor.Extract(ctx, assetName, assetData, []string{expandedGlob})
	if err != nil {
		return nil, fmt.Errorf("embedded: extract checksum file: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("embedded: no file matched traversal glob %q inside %q", expandedGlob, assetName)
	}
	sums, err := parseSha256Sums(files[0].Data)
	if err != nil {
		return nil, fmt.Errorf("embedded: parse checksum file: %w", err)
	}
	hex, ok := sums[assetName]
	if !ok {
		return nil, fmt.Errorf("embedded: asset %q not found in embedded checksum file", assetName)
	}
	return map[string]string{"sha-256": hex}, nil
}
