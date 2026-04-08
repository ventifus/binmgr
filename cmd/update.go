/*
Copyright © 2023 Andrew Denton <ventifus@flying-snail.net>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program. If not, see <http://www.gnu.org/licenses/>.
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/ventifus/binmgr/pkg/manager"
)

var updatePin bool
var updateUnpin bool

var updateCmd = &cobra.Command{
	Use:   "update [PACKAGE[@VERSION]...] [flags]",
	Short: "Update installed packages to their latest versions",
	Long:  `Update installed packages. With no arguments, updates all non-pinned packages. Named packages are always updated, even if pinned.`,
	RunE:  runUpdate,
}

func runUpdate(cmd *cobra.Command, args []string) error {
	if updatePin && updateUnpin {
		return fmt.Errorf("cannot use --pin and --unpin together")
	}
	if (updatePin || updateUnpin) && len(args) == 0 {
		return fmt.Errorf("--pin/--unpin require at least one package name")
	}

	var targets []manager.PackageTarget
	for _, arg := range args {
		id, version := parsePackageAtVersion(arg)
		targets = append(targets, manager.PackageTarget{ID: id, Version: version})
	}

	opts := manager.UpdateOptions{
		Packages: targets,
		Pin:      updatePin,
		Unpin:    updateUnpin,
		OnUpgrade: func(id, oldVersion, newVersion string) {
			fmt.Printf("%s  %s → %s\n", id, oldVersion, newVersion)
		},
	}

	results, err := mgr.Update(cmd.Context(), opts)
	if err != nil {
		return err
	}

	var failed int
	for _, result := range results {
		switch {
		case result.Err != nil:
			fmt.Fprintf(cmd.ErrOrStderr(), "error: %s: %v\n", result.ID, result.Err)
			failed++
		case !result.Updated:
			fmt.Printf("%s  %s  up to date\n", result.ID, result.OldVersion)
		}
	}

	if failed > 0 {
		return fmt.Errorf("%d package(s) failed to update", failed)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(updateCmd)
	updateCmd.Flags().BoolVar(&updatePin, "pin", false, "Pin each named package at the version it is updated to")
	updateCmd.Flags().BoolVar(&updateUnpin, "unpin", false, "Remove the pin from each named package, then update to latest")
}
