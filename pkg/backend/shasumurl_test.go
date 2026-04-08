package backend

import (
	"net/url"
	"testing"
)

func TestShasumUrlManifestCreation(t *testing.T) {
	u, err := url.Parse("https://example.com/path/to/checksums.txt")
	if err != nil {
		t.Fatal(err)
	}

	m := NewBinmgrManifest()
	m.Type = "shasumurl"
	m.Name = u.String()
	m.LatestRemoteUrl = u.String()

	if m.Type != "shasumurl" {
		t.Errorf("Expected type 'shasumurl', got %s", m.Type)
	}
	if m.LatestRemoteUrl != u.String() {
		t.Errorf("LatestRemoteUrl not set correctly")
	}
}

func TestShasumUrlManifestFileName(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantPart string
	}{
		{
			name:     "simple url",
			url:      "https://example.com/checksums.txt",
			wantPart: "shasumurl_",
		},
		{
			name:     "url with path",
			url:      "https://example.com/path/to/checksums.txt",
			wantPart: "shasumurl_",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, _ := url.Parse(tt.url)
			manifestName := "shasumurl_" + u.String()

			if !containsMiddle(manifestName, tt.wantPart) {
				t.Errorf("Manifest name doesn't contain %s", tt.wantPart)
			}
		})
	}
}
