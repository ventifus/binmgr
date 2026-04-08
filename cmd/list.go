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
	"os"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Show all installed packages",
	Long:  `List all installed packages, their versions, and installed file paths.`,
	RunE:  runList,
}

func runList(cmd *cobra.Command, args []string) error {
	packages, err := mgr.List(context.Background())
	if err != nil {
		return err
	}

	for i, pkg := range packages {
		pinnedStr := ""
		if pkg.Pinned {
			pinnedStr = "  [pinned]"
		}
		fmt.Printf("%-50s %s%s\n", pkg.ID, pkg.Version, pinnedStr)
		for _, spec := range pkg.Specs {
			for _, f := range spec.InstalledFiles {
				fmt.Fprintf(os.Stdout, "  %s\n", f.LocalPath)
			}
		}
		if i < len(packages)-1 {
			fmt.Println()
		}
	}

	return nil
}

func init() {
	rootCmd.AddCommand(listCmd)
}
