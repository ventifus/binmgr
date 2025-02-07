/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"time"

	"github.com/go-errors/errors"
	"github.com/spf13/cobra"
	"github.com/ventifus/binmgr/pkg/backend"
	"golang.org/x/exp/slices"
)

// installCmd represents the install command
var installCmd = &cobra.Command{
	Use:   "install [URL...]",
	Short: "Installs a binary",
	Long:  `Installs binaries found at one or more URLs.`,
	Args:  validate,
	RunE:  install,
}

func getPackageTypes() []string {
	return []string{"github", "tarball", "shasumurl"}
}

func validate(cmd *cobra.Command, args []string) error {
	if err := cobra.MinimumNArgs(1)(cmd, args); err != nil {
		return err
	}
	if val := cmd.Flag("type").Value.String(); !slices.Contains(getPackageTypes(), val) {
		return errors.Errorf("unsupported type %s", val)
	}
	return nil
}

func install(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), time.Minute*5)
	defer cancel()
	fileGlob := cmd.Flag("file").Value.String()
	outFile := cmd.Flag("outfile").Value.String()
	remoteType := cmd.Flag("type").Value.String()

	if remoteType == "github" {
		return backend.InstallGithub(ctx, args[0], fileGlob, outFile)
	} else if remoteType == "shasumurl" {
		return backend.InstallShasumUrl(ctx, args[0], fileGlob, outFile)
	}
	return errors.Errorf("unsupported type")
}

func init() {
	rootCmd.AddCommand(installCmd)
	installCmd.Flags().String("type", "github", "Type of package")
	installCmd.Flags().String("file", "", "If there are multiple files, select file name to install")
	installCmd.MarkFlagRequired("file")
	installCmd.Flags().String("outfile", "", "The local file name")
	installCmd.Flags().String("xform", "", "Transform file names with regex")
}
