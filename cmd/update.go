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
var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	RunE: update,
}

func update(cmd *cobra.Command, args []string) error {
	// log := log.WithField("command", "list")
	ctx, cancel := context.WithTimeout(cmd.Context(), time.Minute*5)
	defer cancel()
	if len(args) == 0 {
		return updateAll(ctx)
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
		if m.Type == "github" {
			err = backend.UpdateGithub(ctx, m)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
