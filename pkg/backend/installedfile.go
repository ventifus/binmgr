package backend

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"os"
	"path"

	"github.com/apex/log"
	"github.com/go-errors/errors"
	"github.com/h2non/filetype"
)

const (
	createFailed = "failed to create file"
	copyFailed   = "failed to copy file"
	chmodFailed  = "failed to set file mode"
)

func InstallFile(artifact *Artifact, file []byte, localFile string) error {
	log := log.WithField("localfile", localFile)
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
	log.WithField("mime_type", kind.MIME.Type).WithField("mime_subtype", kind.MIME.Subtype).WithField("mime_value", kind.MIME.Value).Debug("file info")

	var ia *InnerArtifact
	for _, i := range artifact.InnerArtifacts {
		if i.LocalFile == localFile {
			ia = i
		}
	}

	// Progressively uncompress/extract files to handle nested tar.gzip
	if kind.MIME.Value == "application/gzip" {
		log.Debug("decompressing gzip")
		file, err = decompress(file)
		if err != nil {
			return err
		}
		// update kind with the inner file
		kind, err = filetype.Match(file)
		if err != nil {
			return err
		}
		log.Debugf("gzip contents is %s", kind.MIME.Value)
	}
	if kind.MIME.Value == "application/x-tar" {
		log.Debug("extracting tar")
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
				newInnerArtifact := false
				if ia == nil {
					newInnerArtifact = true
					ia = NewInnerArtifact()
				}
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
				if newInnerArtifact {
					artifact.InnerArtifacts = append(artifact.InnerArtifacts, ia)
				}
				kind, err = filetype.Match(innerfile)
				if err != nil {
					return err
				}
				log.Debugf("inner file \"%s\" is %s", hdr.Name, kind.MIME.Value)

				if kind.MIME.Value == "application/x-executable" {
					ia.LocalFile = localFile
					err := installBin(innerfile, localFile)
					if err != nil {
						return err
					}
					ia.Installed = true
					return nil
				}
			}
		}
		return errors.Errorf("no matching files in tar")
	} else if kind.MIME.Value == "application/x-executable" || kind.MIME.Value == "" {
		// Assume an object with no MIME type is a binary file.
		artifact.LocalFile = localFile
		err = installBin(file, localFile)
		if err != nil {
			return err
		}
		artifact.Installed = true
		return nil
	}
	return errors.Errorf("can't install file type \"%s\"", kind.MIME.Value)
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
	log := log.WithField("f", "DownloadFile").WithField("url", artifact.RemoteFile)
	log.Debug("downloading file")
	if client == nil {
		client = &http.Client{}
	}
	resp, err := client.Get(artifact.RemoteFile)
	if err != nil {
		log.WithError(err).Error("failed to download")
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.Errorf("%s", resp.Status)
	}
	file, err := io.ReadAll(resp.Body)
	if err != nil {
		log.WithError(err).Error("failed to read response body")
		return nil, err
	}
	return file, nil
}
