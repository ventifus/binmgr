package backend

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/apex/log"
)

const (
	libPath = ".local/share/binmgr/"
)

type Artifact struct {
	LocalFile      string           `json:"local_file"`
	AssetUrl       string           `json:"asset"`
	RemoteFile     string           `json:"remote_file"`
	Checksums      []string         `json:"checksums"`
	Installed      bool             `json:"installed"`
	FromGlob       string           `json:"from_glob"`
	InnerArtifacts []*InnerArtifact `json:"inner_artifacts"`
}

type InnerArtifact struct {
	FromGlob   string   `json:"from_glob"`
	Checksums  []string `json:"checksums"`
	Installed  bool     `json:"installed"`
	SourcePath string   `json:"source"`
	LocalFile  string   `json:"local_file"`
}

type BinmgrManifest struct {
	Type             string            `json:"type"`
	Name             string            `json:"name"`
	ManifestFileName string            `json:"-"`
	CurrentVersion   string            `json:"version"`
	CurrentRemoteUrl string            `json:"remote_url"`
	LatestRemoteUrl  string            `json:"latest_url"`
	ChecksumFile     string            `json:"checksum_file"`
	Artifacts        []*Artifact       `json:"artifacts"`
	Properties       map[string]string `json:"properties"`
}

func NewArtifact() *Artifact {
	return &Artifact{
		Checksums:      make([]string, 0),
		InnerArtifacts: make([]*InnerArtifact, 0),
	}
}

func NewInnerArtifact() *InnerArtifact {
	return &InnerArtifact{}
}

func NewBinmgrManifest() *BinmgrManifest {
	return &BinmgrManifest{
		Artifacts: make([]*Artifact, 0),
	}
}

func (m BinmgrManifest) String() string {
	return fmt.Sprintf("name=%s currentversion=%s currentremoteurl=%s latestremoteurl=%s artifacts=%v", m.Name, m.CurrentVersion, m.CurrentRemoteUrl, m.LatestRemoteUrl, m.Artifacts)
}

func libDir() string {
	return path.Join(os.Getenv("HOME"), libPath)
}

func (m BinmgrManifest) SaveManifest() error {
	log := log.WithField("f", "SaveManifest")
	fPath := path.Join(libDir(), m.ManifestFileName)
	log = log.WithField("path", fPath)
	f, err := os.Create(fPath)
	if err != nil {
		log.WithError(err).Error(createFailed)
		return err
	}
	defer f.Close()

	b, err := json.MarshalIndent(m, "", "    ")
	if err != nil {
		log.WithError(err).Error("failed to marshal json")
		return err
	}

	d, err := f.Write(b)
	if err != nil {
		log.WithError(err).Error("failed to write file")
	} else {
		log.WithField("size", d).Debug("wrote manifest")
	}
	return err
}

func GetAllManifests() ([]*BinmgrManifest, error) {
	lib, err := os.ReadDir(libDir())
	if err != nil {
		return nil, err
	}
	manifests := make([]*BinmgrManifest, len(lib))
	for i, de := range lib {
		if !de.Type().IsRegular() {
			continue
		}
		fileName := path.Join(libDir(), de.Name())
		f, err := os.Open(fileName)
		if err != nil {
			log.WithError(err).Error("could not open file")
			continue
		}
		defer f.Close()

		b, err := io.ReadAll(f)
		if err != nil {
			log.WithError(err).Error("could not read file")
			return nil, err
		}

		manifests[i] = &BinmgrManifest{}
		err = json.Unmarshal(b, manifests[i])
		if err != nil {
			return nil, err
		}
		manifests[i].ManifestFileName = path.Base(fileName)
	}
	return manifests, nil
}
