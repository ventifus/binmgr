/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"time"

	"github.com/k0kubun/pp/v3"
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
	if loglevel == "debug" {
		pp.Println(manifests)
	}
	for _, m := range manifests {
		if m.Type == "github" {
			backend.GithubStatus(ctx, m)
		} else if m.Type == "shasumurl" {
			backend.ShasumUrlStatus(ctx, m)
		}
	}
	return nil
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
