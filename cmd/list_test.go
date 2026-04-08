package cmd

import (
	"testing"
)

// List command has no flags to validate; integration tests are in pkg/manager.
func TestListCmd_CommandRegistered(t *testing.T) {
	found := false
	for _, sub := range rootCmd.Commands() {
		if sub.Use == "list" {
			found = true
			break
		}
	}
	if !found {
		t.Error("list command not registered on rootCmd")
	}
}
