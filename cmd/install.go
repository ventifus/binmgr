/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/apex/log"
	"github.com/go-errors/errors"
	"github.com/spf13/cobra"
	"github.com/ventifus/binmgr/pkg/backend"
	"golang.org/x/exp/slices"
)

// installCmd represents the install command
var installCmd = &cobra.Command{
	Use:   "install [URL]",
	Short: "Installs a binary",
	Long:  `Installs binaries found at one or more URLs.`,
	Args:  installArgs,
	RunE:  install,
}

func getPackageTypes() []string {
	return []string{"github", "tarball", "shasumurl", "kubefile"}
}

func installArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.MinimumNArgs(1)(cmd, args); err != nil {
		log.WithError(err).Error("invalid number of arguments")
		return err
	}
	if val := cmd.Flag("type").Value.String(); !slices.Contains(getPackageTypes(), val) {
		log.Error("unsupported type")
		return errors.Errorf("unsupported type %s", val)
	}
	if val := strings.Split(cmd.Flag("checksumtype").Value.String(), "!")[0]; !slices.Contains(backend.ChecksumTypes(), val) {
		log.Error("unsupported checksum type")
		return errors.Errorf("unsupported checksum type %s", val)
	}
	return nil
}

func install(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), time.Minute*5)
	defer cancel()
	fileGlob := cmd.Flag("file").Value.String()
	outFile := cmd.Flag("outfile").Value.String()
	remoteType := cmd.Flag("type").Value.String()
	checksumType := cmd.Flag("checksumtype").Value.String()
	remoteUrlHttps := args[0]
	if !strings.Contains(remoteUrlHttps, "://") {
		remoteUrlHttps = fmt.Sprintf("https://%s", remoteUrlHttps)
	}
	remoteUrl, err := url.Parse(remoteUrlHttps)
	if err != nil {
		log.WithError(err).Error("failed to parse url")
	}
	if remoteUrl.Host == "github.com" {
		log.WithField("remoteUrl", remoteUrl).WithField("originalRemoteType", remoteType).Debug("setting remote type to github")
		remoteType = "github"
	}
	if remoteUrl.Host == "dl.k8s.io" {
		log.WithField("remoteUrl", remoteUrl).WithField("originalRemoteType", remoteType).Debug("setting remote type to kubeurl")
		remoteType = "kubeurl"
	}

	log := log.WithFields(log.Fields{
		"remoteUrl":  remoteUrl,
		"remoteType": remoteType,
		"fileGlob":   fileGlob,
		"outFile":    outFile,
	})
	log.Debug("attempting install")

	switch remoteType {
	case "github":
		return backend.InstallGithub(ctx, remoteUrl, fileGlob, outFile, checksumType)
	case "kubeurl":
		return backend.InstallKubeFile(ctx, remoteUrl, fileGlob, outFile)
	case "shasumurl":
		return backend.InstallShasumUrl(ctx, remoteUrl, fileGlob, outFile)
	}

	return errors.Errorf("unsupported type")
}

func init() {
	rootCmd.AddCommand(installCmd)
	installCmd.Flags().String("type", "github", "Type of package")
	installCmd.Flags().String("file", "", "If there are multiple files, select file name to install. Separate inner file names with an exclamation point `!`.")
	installCmd.Flags().String("outfile", "", "The local file name")
	installCmd.Flags().String("xform", "", "Transform file names with regex")
	installCmd.Flags().String("checksumtype", "sha256sums", fmt.Sprintf(
		"Type of checksum to use [%s]", strings.Join(backend.ChecksumTypes(), ","),
	))
}
