package cmd

import (
	"testing"
)

// Status command has no flags to validate; integration tests are in pkg/manager.
func TestStatusCmd_CommandRegistered(t *testing.T) {
	found := false
	for _, sub := range rootCmd.Commands() {
		if sub.Use == "status [PACKAGE...]" {
			found = true
			break
		}
	}
	if !found {
		t.Error("status command not registered on rootCmd")
	}
}
