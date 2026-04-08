package backend

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetUrlAsString(t *testing.T) {
	tests := []struct {
		name       string
		response   string
		statusCode int
		want       string
		wantErr    bool
	}{
		{
			name:       "successful request",
			response:   "v1.28.0",
			statusCode: 200,
			want:       "v1.28.0",
			wantErr:    false,
		},
		{
			name:       "404 error",
			response:   "Not Found",
			statusCode: 404,
			wantErr:    true,
		},
		{
			name:       "empty response",
			response:   "",
			statusCode: 200,
			want:       "",
			wantErr:    false,
		},
		{
			name:       "multiline response",
			response:   "line1\nline2\nline3",
			statusCode: 200,
			want:       "line1\nline2\nline3",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.response))
			}))
			defer server.Close()

			got, err := getUrlAsString(nil, server.URL)
			if (err != nil) != tt.wantErr {
				t.Errorf("getUrlAsString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && got != tt.want {
				t.Errorf("getUrlAsString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetUrlAsString_WithClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("test"))
	}))
	defer server.Close()

	client := &http.Client{}
	got, err := getUrlAsString(client, server.URL)
	if err != nil {
		t.Errorf("getUrlAsString() with custom client failed: %v", err)
	}
	if got != "test" {
		t.Errorf("getUrlAsString() = %q, want %q", got, "test")
	}
}

func TestGetUrlAsString_InvalidURL(t *testing.T) {
	_, err := getUrlAsString(nil, "http://invalid-url-that-does-not-exist-12345.com")
	if err == nil {
		t.Error("Expected error for invalid URL")
	}
}
