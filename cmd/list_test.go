package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/ventifus/binmgr/pkg/manager"
	"github.com/ventifus/binmgr/pkg/manifest"
)

// mockManager implements manager.Manager with configurable function fields.
type mockManager struct {
	InstallFn   func(ctx context.Context, opts manager.InstallOptions) error
	UpdateFn    func(ctx context.Context, opts manager.UpdateOptions) ([]*manager.UpdateResult, error)
	StatusFn    func(ctx context.Context, packages []string) ([]*manager.StatusResult, error)
	ListFn      func(ctx context.Context) ([]*manifest.Package, error)
	UninstallFn func(ctx context.Context, packages []string) error
}

func (m *mockManager) Install(ctx context.Context, opts manager.InstallOptions) error {
	return m.InstallFn(ctx, opts)
}

func (m *mockManager) Update(ctx context.Context, opts manager.UpdateOptions) ([]*manager.UpdateResult, error) {
	return m.UpdateFn(ctx, opts)
}

func (m *mockManager) Status(ctx context.Context, packages []string) ([]*manager.StatusResult, error) {
	return m.StatusFn(ctx, packages)
}

func (m *mockManager) List(ctx context.Context) ([]*manifest.Package, error) {
	return m.ListFn(ctx)
}

func (m *mockManager) Uninstall(ctx context.Context, packages []string) error {
	return m.UninstallFn(ctx, packages)
}

// captureStdout replaces os.Stdout with a pipe and returns a function that
// restores os.Stdout and returns all bytes written during the capture.
func captureStdout(t *testing.T) func() string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = orig })
	return func() string {
		w.Close()
		var buf bytes.Buffer
		io.Copy(&buf, r) //nolint:errcheck
		r.Close()
		return buf.String()
	}
}

func TestRunList(t *testing.T) {
	tests := []struct {
		name     string
		packages []*manifest.Package
		wantIn   []string
		wantOut  []string // strings that must NOT appear
	}{
		{
			name:     "no packages",
			packages: []*manifest.Package{},
			wantIn:   []string{},
		},
		{
			name: "one package",
			packages: []*manifest.Package{
				{
					ID:      "github.com/casey/just",
					Version: "1.40.0",
					Specs:   []manifest.InstallSpec{},
				},
			},
			wantIn: []string{"github.com/casey/just", "1.40.0"},
		},
		{
			name: "two packages",
			packages: []*manifest.Package{
				{
					ID:      "github.com/casey/just",
					Version: "1.40.0",
					Specs:   []manifest.InstallSpec{},
				},
				{
					ID:      "github.com/cli/cli",
					Version: "2.50.0",
					Specs:   []manifest.InstallSpec{},
				},
			},
			wantIn: []string{
				"github.com/casey/just", "1.40.0",
				"github.com/cli/cli", "2.50.0",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orig := mgr
			t.Cleanup(func() { mgr = orig })

			mgr = &mockManager{
				ListFn: func(ctx context.Context) ([]*manifest.Package, error) {
					return tc.packages, nil
				},
			}

			flush := captureStdout(t)
			err := runList(&cobra.Command{}, []string{})
			got := flush()

			if err != nil {
				t.Fatalf("runList returned error: %v", err)
			}

			for _, want := range tc.wantIn {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\ngot:\n%s", want, got)
				}
			}
		})
	}
}

func TestRunList_InstalledFiles(t *testing.T) {
	orig := mgr
	t.Cleanup(func() { mgr = orig })

	mgr = &mockManager{
		ListFn: func(ctx context.Context) ([]*manifest.Package, error) {
			return []*manifest.Package{
				{
					ID:      "github.com/casey/just",
					Version: "1.40.0",
					Specs: []manifest.InstallSpec{
						{
							InstalledFiles: []manifest.InstalledFile{
								{LocalPath: "/home/user/.local/bin/just"},
							},
						},
					},
				},
			}, nil
		},
	}

	flush := captureStdout(t)
	err := runList(&cobra.Command{}, []string{})
	got := flush()

	if err != nil {
		t.Fatalf("runList returned error: %v", err)
	}

	if !strings.Contains(got, "/home/user/.local/bin/just") {
		t.Errorf("output missing installed file path\ngot:\n%s", got)
	}
}

func TestRunList_PinnedMarker(t *testing.T) {
	orig := mgr
	t.Cleanup(func() { mgr = orig })

	mgr = &mockManager{
		ListFn: func(ctx context.Context) ([]*manifest.Package, error) {
			return []*manifest.Package{
				{
					ID:      "github.com/casey/just",
					Version: "1.40.0",
					Pinned:  true,
					Specs:   []manifest.InstallSpec{},
				},
			}, nil
		},
	}

	flush := captureStdout(t)
	err := runList(&cobra.Command{}, []string{})
	got := flush()

	if err != nil {
		t.Fatalf("runList returned error: %v", err)
	}

	if !strings.Contains(got, "[pinned]") {
		t.Errorf("output missing [pinned] marker\ngot:\n%s", got)
	}
}
