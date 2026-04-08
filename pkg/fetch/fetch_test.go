package fetch

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPFetcher_Success(t *testing.T) {
	want := []byte("hello from the test server")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(want)))
		w.WriteHeader(http.StatusOK)
		w.Write(want)
	}))
	defer srv.Close()

	f := NewFetcher()
	got, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch returned unexpected error: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("body mismatch: got %q, want %q", got, want)
	}
}

func TestHTTPFetcher_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer srv.Close()

	f := NewFetcher()
	_, err := f.Fetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected an error for non-200 status, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected error to contain status code 404, got: %v", err)
	}
}
