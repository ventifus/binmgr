/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/ventifus/binmgr/pkg/backend"
)

// uninstallCmd represents the uninstall command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Checks for updates and prints if binaries need updating",
	Long:  `Prints the status of each binary, indicating if updates are needed`,
	RunE:  status,
}

func status(cmd *cobra.Command, args []string) error {
	// log := log.WithField("command", "list")
	ctx, cancel := context.WithTimeout(cmd.Context(), time.Minute*5)
	defer cancel()
	manifests, err := backend.GetAllManifests()
	if err != nil {
		return err
	}
	for _, m := range manifests {
		var err error
		if m.Type == "github" {
			err = backend.GithubStatus(ctx, m)
		} else if m.Type == "shasumurl" {
			err = backend.ShasumUrlStatus(ctx, m)
		} else if m.Type == "kubeurl" {
			err = backend.KubeUrlStatus(ctx, m)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
