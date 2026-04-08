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

var statusCmd = &cobra.Command{
	Use:   "status [PACKAGE...]",
	Short: "Report whether newer versions are available",
	Long:  `Check installed packages for available updates. With no arguments, checks all installed packages.`,
	Run:   runStatus,
}

func runStatus(cmd *cobra.Command, args []string) {
	results, err := mgr.Status(context.Background(), args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	anyUpdates := false
	for _, result := range results {
		pinnedStr := ""
		if result.Pinned {
			pinnedStr = "  [pinned]"
		}
		if result.UpdateAvailable {
			anyUpdates = true
			fmt.Printf("%-50s %-20s → %-20s%s\n", result.ID, result.InstalledVersion, result.LatestVersion, pinnedStr)
		} else {
			fmt.Printf("%-50s %-20s up to date%s\n", result.ID, result.InstalledVersion, pinnedStr)
		}
	}

	if anyUpdates {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
