/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/apex/log"
	"github.com/spf13/cobra"

	"github.com/ventifus/binmgr/pkg/backend"
)

// uninstallCmd represents the uninstall command
var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Updates all installed binaries",
	Long:  `Checks the status of each installed binary and updates each one to the latest version`,
	RunE:  update,
}

func update(cmd *cobra.Command, args []string) error {
	// log := log.WithField("command", "list")
	ctx, cancel := context.WithTimeout(cmd.Context(), time.Minute*5)
	defer cancel()
	if len(args) == 0 {
		return updateAll(ctx)
	}
	manifests, err := backend.GetAllManifests()
	if err != nil {
		return err
	}
	for _, pkg := range args {
		for _, m := range manifests {
			if m.Name == pkg {
				err = updatePackage(ctx, m)
				if err != nil {
					log.WithError(err).Debug("update package failed")
					fmt.Printf("Error: %v", err)
				}
			}
		}
	}
	return nil
}

func updatePackage(ctx context.Context, m *backend.BinmgrManifest) error {
	if m.Type == "github" {
		return backend.UpdateGithub(ctx, m)

	} else if m.Type == "shasumurl" {
		return backend.UpdateShasumUrl(ctx, m)
	}
	return nil
}

func updateAll(ctx context.Context) error {
	manifests, err := backend.GetAllManifests()
	if err != nil {
		return err
	}
	// w := tabwriter.NewWriter(os.Stdout, 0, 4, 4, ' ', 0x0)
	// defer w.Flush()
	for _, m := range manifests {
		err = updatePackage(ctx, m)
		if err != nil {
			log.WithError(err).Debug("update package failed")
			fmt.Printf("Error: %v", err)
		}
	}
	return nil
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
