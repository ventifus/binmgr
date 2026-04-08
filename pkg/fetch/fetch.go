package fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/schollz/progressbar/v3"
)

// Fetcher downloads the content at a URL and returns it as bytes.
type Fetcher interface {
	Fetch(ctx context.Context, url string) ([]byte, error)
}

// HTTPFetcher is an unauthenticated HTTP fetcher that shows a progress bar on
// stderr while downloading.
type HTTPFetcher struct {
	client *http.Client
}

// NewFetcher returns a new Fetcher backed by a default HTTP client.
func NewFetcher() Fetcher {
	return &HTTPFetcher{
		client: &http.Client{},
	}
}

// Fetch downloads url and returns its body as a byte slice. It displays a
// progress bar on stderr during the download. A non-200 status code is
// returned as an error.
func (f *HTTPFetcher) Fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	bar := progressbar.DefaultBytes(resp.ContentLength, "downloading")
	body, err := io.ReadAll(io.TeeReader(resp.Body, bar))
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	return body, nil
}
