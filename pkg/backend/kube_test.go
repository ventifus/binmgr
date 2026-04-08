package backend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ventifus/binmgr/pkg/manifest"
)

func TestKubeBackend_CanHandle(t *testing.T) {
	tests := []struct {
		name string
		host string
		want bool
	}{
		{
			name: "dl.k8s.io returns true",
			host: "dl.k8s.io",
			want: true,
		},
		{
			name: "github.com returns false",
			host: "github.com",
			want: false,
		},
		{
			name: "mirror.openshift.com returns false",
			host: "mirror.openshift.com",
			want: false,
		},
	}

	b := NewKubeBackend()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &url.URL{Host: tt.host}
			got := b.CanHandle(u)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestKubeBackend_Type(t *testing.T) {
	b := NewKubeBackend()
	if got := b.Type(); got != "kubeurl" {
		t.Errorf("Type() = %q, want %q", got, "kubeurl")
	}
}

func TestKubeBackend_Resolve_FetchesVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "binmgr" {
			t.Errorf("expected User-Agent: binmgr, got %q", r.Header.Get("User-Agent"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("v1.35.0\n"))
	}))
	defer server.Close()

	b := &kubeBackend{client: server.Client()}
	sourceURL, _ := url.Parse(server.URL)
	res, err := b.Resolve(context.Background(), sourceURL, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if res.Version != "v1.35.0" {
		t.Errorf("Resolve() Version = %q, want %q", res.Version, "v1.35.0")
	}
	if res.Assets != nil {
		t.Errorf("Resolve() Assets = %v, want nil", res.Assets)
	}
}

func TestKubeBackend_Resolve_TrimsWhitespace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("  v1.35.0  \n"))
	}))
	defer server.Close()

	b := &kubeBackend{client: server.Client()}
	sourceURL, _ := url.Parse(server.URL)
	res, err := b.Resolve(context.Background(), sourceURL, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if res.Version != "v1.35.0" {
		t.Errorf("Resolve() Version = %q, want %q (whitespace not trimmed)", res.Version, "v1.35.0")
	}
}

func TestKubeBackend_Resolve_OptsVersionSkipsHTTP(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("v1.35.0\n"))
	}))
	defer server.Close()

	b := &kubeBackend{client: server.Client()}
	sourceURL, _ := url.Parse(server.URL)
	res, err := b.Resolve(context.Background(), sourceURL, ResolveOptions{Version: "v1.28.0"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if res.Version != "v1.28.0" {
		t.Errorf("Resolve() Version = %q, want %q", res.Version, "v1.28.0")
	}
	if requestCount != 0 {
		t.Errorf("Resolve() made %d HTTP requests, want 0 when opts.Version is set", requestCount)
	}
	if res.Assets != nil {
		t.Errorf("Resolve() Assets = %v, want nil", res.Assets)
	}
}

func TestKubeBackend_Resolve_NonOKStatusReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not Found"))
	}))
	defer server.Close()

	b := &kubeBackend{client: server.Client()}
	sourceURL, _ := url.Parse(server.URL)
	_, err := b.Resolve(context.Background(), sourceURL, ResolveOptions{})
	if err == nil {
		t.Error("Resolve() expected error for non-200 response, got nil")
	}
}

func TestKubeBackend_Check_ReturnsFreshVersion(t *testing.T) {
	version := "v1.35.0"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(version + "\n"))
	}))
	defer server.Close()

	b := &kubeBackend{client: server.Client()}
	pkg := &manifest.Package{
		SourceURL: server.URL,
		Version:   "v1.34.0",
	}

	res, err := b.Check(context.Background(), pkg)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "v1.35.0" {
		t.Errorf("Check() Version = %q, want %q", res.Version, "v1.35.0")
	}
	if res.Assets != nil {
		t.Errorf("Check() Assets = %v, want nil", res.Assets)
	}
}

func TestKubeBackend_Check_UpdatedVersionWhenServerChanges(t *testing.T) {
	currentResponse := "v1.34.0"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(currentResponse))
	}))
	defer server.Close()

	b := &kubeBackend{client: server.Client()}
	pkg := &manifest.Package{
		SourceURL: server.URL,
		Version:   "v1.34.0",
	}

	res1, err := b.Check(context.Background(), pkg)
	if err != nil {
		t.Fatalf("first Check() error = %v", err)
	}
	if res1.Version != "v1.34.0" {
		t.Errorf("first Check() Version = %q, want %q", res1.Version, "v1.34.0")
	}

	// Simulate server updating the stable pointer
	currentResponse = "v1.35.0"

	res2, err := b.Check(context.Background(), pkg)
	if err != nil {
		t.Fatalf("second Check() error = %v", err)
	}
	if res2.Version != "v1.35.0" {
		t.Errorf("second Check() Version = %q, want %q", res2.Version, "v1.35.0")
	}
}

func TestKubeBackend_Check_NonOKStatusReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	b := &kubeBackend{client: server.Client()}
	pkg := &manifest.Package{
		SourceURL: server.URL,
		Version:   "v1.34.0",
	}

	_, err := b.Check(context.Background(), pkg)
	if err == nil {
		t.Error("Check() expected error for non-200 response, got nil")
	}
}
