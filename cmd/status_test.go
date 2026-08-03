package cmd

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/ventifus/binmgr/pkg/manager"
)

// newCmd returns a cobra.Command with a background context set,
// suitable for passing to runStatus in tests.
func newCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	return cmd
}

// mockManager is defined in list_test.go (same package).

// TestRunStatus_UpToDate verifies that a package with no update available
// prints "up to date" and does not include an arrow (→).
func TestRunStatus_UpToDate(t *testing.T) {
	orig := mgr
	t.Cleanup(func() { mgr = orig })

	mgr = &mockManager{
		StatusFn: func(ctx context.Context, packages []string) ([]*manager.StatusResult, error) {
			return []*manager.StatusResult{
				{
					ID:               "github.com/casey/just",
					InstalledVersion: "1.40.0",
					LatestVersion:    "1.40.0",
					Pinned:           false,
					UpdateAvailable:  false,
				},
			}, nil
		},
	}

	cmd := newCmd()
	flush := captureStdout(t)
	// runStatus calls os.Exit(1) only when anyUpdates is true; safe to call directly here.
	runStatus(cmd, []string{})
	got := flush()

	if !strings.Contains(got, "up to date") {
		t.Errorf("expected 'up to date' in output\ngot:\n%s", got)
	}
	if !strings.Contains(got, "github.com/casey/just") {
		t.Errorf("expected package ID in output\ngot:\n%s", got)
	}
	if strings.Contains(got, "→") {
		t.Errorf("did not expect update arrow in up-to-date output\ngot:\n%s", got)
	}
}

// TestRunStatus_Pinned verifies that a pinned package shows "[pinned]" in output.
func TestRunStatus_Pinned(t *testing.T) {
	orig := mgr
	t.Cleanup(func() { mgr = orig })

	mgr = &mockManager{
		StatusFn: func(ctx context.Context, packages []string) ([]*manager.StatusResult, error) {
			return []*manager.StatusResult{
				{
					ID:               "github.com/casey/just",
					InstalledVersion: "1.40.0",
					LatestVersion:    "1.40.0",
					Pinned:           true,
					UpdateAvailable:  false,
				},
			}, nil
		},
	}

	cmd := newCmd()
	flush := captureStdout(t)
	runStatus(cmd, []string{})
	got := flush()

	if !strings.Contains(got, "[pinned]") {
		t.Errorf("expected '[pinned]' marker in output\ngot:\n%s", got)
	}
}

// TestRunStatus_UpdateAvailable runs in a subprocess because runStatus calls
// os.Exit(1) when an update is available. The subprocess prints output to
// stdout before exiting; the parent captures and checks it.
func TestRunStatus_UpdateAvailable(t *testing.T) {
	const marker = "STATUS_UPDATE_AVAILABLE_SUBPROCESS"

	// Subprocess branch: set up the mock and run the real command.
	if os.Getenv(marker) == "1" {
		mgr = &mockManager{
			StatusFn: func(ctx context.Context, packages []string) ([]*manager.StatusResult, error) {
				return []*manager.StatusResult{
					{
						ID:               "github.com/casey/just",
						InstalledVersion: "1.39.0",
						LatestVersion:    "1.40.0",
						Pinned:           false,
						UpdateAvailable:  true,
					},
				}, nil
			},
		}
		cmd := newCmd()
		runStatus(cmd, []string{})
		// runStatus calls os.Exit(1); the line below is unreachable.
		return
	}

	// Parent branch: re-run this test function as a subprocess.
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	cmd := exec.Command(exe, "-test.run=^TestRunStatus_UpdateAvailable$", "-test.v")
	cmd.Env = append(os.Environ(), marker+"=1")

	out, err := cmd.Output()
	// We expect exit code 1 from os.Exit(1).
	if err == nil {
		t.Fatal("expected subprocess to exit with non-zero status")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("unexpected error type: %v", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.ExitCode())
	}

	// cmd.Output() only captures stdout, not stderr. The output we want
	// is the combined stdout of the subprocess.
	got := string(out)
	if !strings.Contains(got, "github.com/casey/just") {
		t.Errorf("expected package ID in subprocess output\ngot:\n%s", got)
	}
	if !strings.Contains(got, "1.39.0") {
		t.Errorf("expected installed version in subprocess output\ngot:\n%s", got)
	}
	if !strings.Contains(got, "1.40.0") {
		t.Errorf("expected latest version in subprocess output\ngot:\n%s", got)
	}
	if !strings.Contains(got, "→") {
		t.Errorf("expected update arrow (→) in subprocess output\ngot:\n%s", got)
	}
}
