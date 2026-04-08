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
	"os"

	"github.com/apex/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/ventifus/binmgr/pkg/backend"
	"github.com/ventifus/binmgr/pkg/extract"
	"github.com/ventifus/binmgr/pkg/fetch"
	"github.com/ventifus/binmgr/pkg/manager"
	"github.com/ventifus/binmgr/pkg/manifest"
	"github.com/ventifus/binmgr/pkg/verify"
)

var cfgFile string
var loglevel string
var mgr manager.Manager

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "binmgr",
	Short: "binmgr installs binaries from various places and can keep them updated",
	Long:  `binmgr installs binaries from various places and can keep them updated`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		log.WithError(err).Error("command execution failed")
		os.Exit(1)
	}
}

func buildRegistry() *backend.Registry {
	r := backend.NewRegistry()
	r.Register(backend.NewGitHubBackend())
	r.Register(backend.NewKubeBackend())
	r.Register(backend.NewShasumBackend())
	return r
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&loglevel, "loglevel", "warn", "Log level")

	mgr = manager.New(
		buildRegistry(),
		fetch.NewFetcher(),
		extract.NewExtractor(),
		verify.NewVerifier(),
		manifest.LibDir(),
	)
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	log.WithField("level", loglevel).Debug("setting log level")
	log.SetLevelFromString(loglevel)
	log.WithField("loglevel", loglevel).Debug("set log level")
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".binmgr" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".binmgr")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	} else {
		log.WithError(err).Debug("no config file found")
	}
}
