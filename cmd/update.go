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
	"context"
	"fmt"
	"strings"

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
		idx := strings.LastIndex(arg, "@")
		if idx >= 0 {
			suffix := arg[idx+1:]
			if len(suffix) > 0 && !strings.Contains(suffix, "/") {
				targets = append(targets, manager.PackageTarget{
					ID:      arg[:idx],
					Version: suffix,
				})
				continue
			}
		}
		targets = append(targets, manager.PackageTarget{ID: arg})
	}

	opts := manager.UpdateOptions{
		Packages: targets,
		Pin:      updatePin,
		Unpin:    updateUnpin,
	}

	results, err := mgr.Update(context.Background(), opts)
	if err != nil {
		return err
	}

	for _, result := range results {
		if result.Updated {
			fmt.Printf("%s  %s → %s\n", result.ID, result.OldVersion, result.NewVersion)
		} else {
			fmt.Printf("%s  %s  up to date\n", result.ID, result.OldVersion)
		}
	}

	return nil
}

func init() {
	rootCmd.AddCommand(updateCmd)
	updateCmd.Flags().BoolVar(&updatePin, "pin", false, "Pin each named package at the version it is updated to")
	updateCmd.Flags().BoolVar(&updateUnpin, "unpin", false, "Remove the pin from each named package, then update to latest")
}
