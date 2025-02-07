/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/k0kubun/pp/v3"
	"github.com/spf13/cobra"

	"github.com/ventifus/binmgr/pkg/backend"
)

// uninstallCmd represents the uninstall command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists all installed binaries",
	Long:  `Generates a list of all binaries and where they came from`,
	RunE:  list,
}

func list(cmd *cobra.Command, args []string) error {
	// log := log.WithField("command", "list")
	_, cancel := context.WithTimeout(cmd.Context(), time.Minute*5)
	defer cancel()
	manifests, err := backend.GetAllManifests()
	if err != nil {
		return err
	}
	if loglevel == "debug" {
		pp.Println(manifests)
	}
	//w := tabwriter.NewWriter(os.Stdout, 0, 4, 4, ' ', 0x0)
	//defer w.Flush()
	for _, m := range manifests {
		fmt.Printf("Package %s %s\n", m.Name, m.CurrentVersion)
		for _, a := range m.Artifacts {
			fmt.Printf("  %s\n", a.RemoteFile)
			if a.Installed {
				fmt.Printf("    - %s\n", path.Base(a.LocalFile))
				//fmt.Fprintf(w, "%s\t%s\t%s\n", path.Base(a.LocalFile), m.CurrentVersion, m.Name)
			}
			for _, i := range a.InnerArtifacts {
				fmt.Printf("    - %s\n", path.Base(i.LocalFile))
				//fmt.Fprintf(w, "%s\t%s\t%s\n", path.Base(i.LocalFile), m.CurrentVersion, m.Name)
			}
		}
	}
	return nil
}

func init() {
	rootCmd.AddCommand(listCmd)
}
