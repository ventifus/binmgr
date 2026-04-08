package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/ventifus/binmgr/pkg/manager"
)

var installCmd = &cobra.Command{
	Use:   "install URL[@VERSION]",
	Short: "Download and install a package",
	Args:  cobra.ExactArgs(1),
	RunE:  runInstall,
}

func init() {
	rootCmd.AddCommand(installCmd)
	installCmd.Flags().StringArrayP("file", "f", nil, "Install spec: ASSET_GLOB[!TRAVERSAL_GLOB...][@LOCAL_NAME] (repeatable; at least one required)")
	installCmd.Flags().String("checksum", "auto", "Checksum strategy (auto|none|shared-file:GLOB|per-asset:SUFFIX|multisum[:DATA[:ORDER]]|embedded:GLOB)")
	installCmd.Flags().String("dir", "", "Default install directory (default: ~/.local/bin/)")
	installCmd.Flags().String("type", "", "Backend override: github | shasumurl | kubeurl")
	installCmd.Flags().Bool("pin", false, "Pin this package to the installed version")
}

// parseURL extracts the URL and optional version from a "URL[@VERSION]" argument.
// The @VERSION suffix is recognised only when the part after the last @ is non-empty
// and contains no '/'.
func parseURL(arg string) (rawURL, version string) {
	idx := strings.LastIndex(arg, "@")
	if idx >= 0 {
		after := arg[idx+1:]
		if after != "" && !strings.Contains(after, "/") {
			version = after
			arg = arg[:idx]
		}
	}
	rawURL = arg
	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}
	return rawURL, version
}

// parseFileSpec parses a single --file value: ASSET_GLOB[!TRAVERSAL_GLOB...][@LOCAL_NAME]
func parseFileSpec(spec string) (assetGlob string, traversalGlobs []string, localName string) {
	// Split on last '@' to extract optional LocalName.
	if idx := strings.LastIndex(spec, "@"); idx >= 0 {
		localName = spec[idx+1:]
		spec = spec[:idx]
	}

	// Expand ~/ in localName.
	if strings.HasPrefix(localName, "~/") {
		home := os.Getenv("HOME")
		localName = home + localName[1:]
	}

	// Split remainder on '!' to get AssetGlob and TraversalGlobs.
	parts := strings.Split(spec, "!")
	assetGlob = parts[0]
	if len(parts) > 1 {
		traversalGlobs = parts[1:]
	}

	return assetGlob, traversalGlobs, localName
}

// parseChecksumStrategy converts a --checksum flag value into a ChecksumOpts.
func parseChecksumStrategy(value string) (manager.ChecksumOpts, error) {
	parts := strings.SplitN(value, ":", 3)
	strategy := parts[0]

	switch strategy {
	case "auto":
		return manager.ChecksumOpts{Strategy: "auto"}, nil

	case "none":
		return manager.ChecksumOpts{Strategy: "none"}, nil

	case "shared-file":
		fileGlob := ""
		if len(parts) >= 2 {
			fileGlob = parts[1]
		}
		return manager.ChecksumOpts{Strategy: "shared-file", FileGlob: fileGlob}, nil

	case "per-asset":
		suffix := ""
		if len(parts) >= 2 {
			suffix = parts[1]
		}
		return manager.ChecksumOpts{Strategy: "per-asset", Suffix: suffix}, nil

	case "multisum":
		dataGlob := "checksums"
		orderGlob := "checksums_hashes_order"
		if len(parts) >= 2 && parts[1] != "" {
			dataGlob = parts[1]
		}
		if len(parts) >= 3 && parts[2] != "" {
			orderGlob = parts[2]
		}
		return manager.ChecksumOpts{Strategy: "multisum", FileGlob: dataGlob, OrderGlob: orderGlob}, nil

	case "embedded":
		traversalGlob := ""
		if len(parts) >= 2 {
			traversalGlob = parts[1]
		}
		return manager.ChecksumOpts{Strategy: "embedded", TraversalGlob: traversalGlob}, nil

	default:
		return manager.ChecksumOpts{}, fmt.Errorf(
			"invalid checksum strategy %q: valid strategies are auto, none, shared-file:GLOB, per-asset:SUFFIX, multisum[:DATA[:ORDER]], embedded:GLOB",
			value,
		)
	}
}

func runInstall(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
	defer cancel()

	// Parse URL[@VERSION].
	sourceURL, version := parseURL(args[0])

	// Parse --file flags.
	fileSpecs, err := cmd.Flags().GetStringArray("file")
	if err != nil {
		return err
	}
	if len(fileSpecs) == 0 {
		return fmt.Errorf("at least one --file flag is required")
	}

	// Parse --checksum.
	checksumValue, err := cmd.Flags().GetString("checksum")
	if err != nil {
		return err
	}
	checksumOpts, err := parseChecksumStrategy(checksumValue)
	if err != nil {
		return err
	}

	// Parse --dir.
	defaultDir, err := cmd.Flags().GetString("dir")
	if err != nil {
		return err
	}
	if defaultDir == "" {
		defaultDir = os.Getenv("HOME") + "/.local/bin/"
	} else if strings.HasPrefix(defaultDir, "~/") {
		defaultDir = os.Getenv("HOME") + defaultDir[1:]
	}

	// Parse --type.
	backendType, err := cmd.Flags().GetString("type")
	if err != nil {
		return err
	}

	// Parse --pin.
	pin, err := cmd.Flags().GetBool("pin")
	if err != nil {
		return err
	}

	// Build SpecOpts for each --file.
	specs := make([]manager.SpecOpts, 0, len(fileSpecs))
	for _, raw := range fileSpecs {
		assetGlob, traversalGlobs, localName := parseFileSpec(raw)
		specs = append(specs, manager.SpecOpts{
			AssetGlob:      assetGlob,
			TraversalGlobs: traversalGlobs,
			LocalName:      localName,
			Checksum:       checksumOpts,
		})
	}

	opts := manager.InstallOptions{
		SourceURL:   sourceURL,
		Version:     version,
		Specs:       specs,
		DefaultDir:  defaultDir,
		BackendType: backendType,
		Pin:         pin,
	}

	if err := mgr.Install(ctx, opts); err != nil {
		return err
	}

	fmt.Printf("Installed %s\n", sourceURL)
	return nil
}
