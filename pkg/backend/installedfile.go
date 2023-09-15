package backend

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"

	"github.com/apex/log"
	"github.com/h2non/filetype"
)

const (
	createFailed = "failed to create file"
	copyFailed   = "failed to copy file"
	chmodFailed  = "failed to set file mode"
)

func InstallFile(artifact *Artifact, file []byte, localFile string) error {
	for _, c := range artifact.Checksums {
		err := VerifyBytes(file, c)
		if err != nil {
			return err
		}
	}
	var err error
	kind, err := filetype.Match(file)
	if err != nil {
		return err
	}
	log.Debugf("file is %s", kind.MIME.Value)

	// Progressively uncompress/extract files to handle nested tar.gzip
	if kind.MIME.Value == "application/gzip" {
		file, err = decompress(file)
		if err != nil {
			return err
		}
		// update kind with the inner file
		kind, err = filetype.Match(file)
		if err != nil {
			return err
		}
		log.Debugf("inner file is %s", kind.MIME.Value)
	}
	if kind.MIME.Value == "application/x-tar" {
		tr := tar.NewReader(bytes.NewReader(file))
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if path.Base(hdr.Name) == path.Base(localFile) {
				ia := NewInnerArtifact()
				ia.LocalFile = localFile
				ia.SourcePath = hdr.Name
				ia.FromGlob = path.Base(localFile)
				innerfile, err := io.ReadAll(tr)
				if err != nil {
					return err
				}
				csum, err := ComputeChecksum(innerfile)
				if err != nil {
					return nil
				}
				ia.Checksums = []string{csum}
				artifact.InnerArtifacts = append(artifact.InnerArtifacts, ia)
				kind, err = filetype.Match(innerfile)
				if err != nil {
					return err
				}
				log.Debugf("inner file \"%s\" is %s", hdr.Name, kind.MIME.Value)

				if kind.MIME.Value == "application/x-executable" {
					err := installBin(innerfile, ia.LocalFile)
					if err != nil {
						return err
					}
					ia.Installed = true
					return nil
				}
			}
		}
		return fmt.Errorf("no matching files in tar")
	} else if kind.MIME.Value == "application/x-executable" {
		err = installBin(file, artifact.LocalFile)
		if err != nil {
			return err
		}
		artifact.Installed = true
		return nil
	}
	return fmt.Errorf("can't install file type %s", kind.MIME.Value)
}

func installBin(file []byte, localFile string) error {
	log := log.WithField("path", localFile)
	f, err := os.Create(localFile)
	if err != nil {
		log.WithError(err).WithField("path", localFile).Error(createFailed)
		return err
	}
	_, err = io.Copy(f, bytes.NewReader(file))
	if err != nil {
		log.WithError(err).WithField("path", localFile).Error(copyFailed)
		return err
	}
	log.WithField("file", localFile).Debug("wrote file")
	err = os.Chmod(localFile, os.FileMode(0755))
	if err != nil {
		log.WithError(err).Errorf(chmodFailed)
	}
	return err
}

func decompress(file []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(file))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(gz)
}

func DownloadFile(ctx context.Context, client *http.Client, artifact *Artifact) ([]byte, error) {
	log := log.WithField("f", "DownloadGithubAsset")
	if client == nil {
		client = &http.Client{}
	}
	resp, err := client.Get(artifact.RemoteFile)
	if err != nil {
		log.WithError(err).WithField("url", artifact.RemoteFile).Error("failed to download")
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s", resp.Status)
	}
	file, err := io.ReadAll(resp.Body)
	if err != nil {
		log.WithError(err).WithField("url", artifact.RemoteFile).Error("failed to read response body")
		return nil, err
	}
	return file, nil
}
