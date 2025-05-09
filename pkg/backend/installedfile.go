package backend

import (
	"archive/tar"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/apex/log"
	"github.com/go-errors/errors"
	"github.com/h2non/filetype"
	"github.com/schollz/progressbar/v3"
)

const (
	createFailed = "failed to create file"
	copyFailed   = "failed to copy file"
	chmodFailed  = "failed to set file mode"
)

func InstallFile(artifact *Artifact, file []byte, localFile string, globs string) error {
	globList := strings.Split(globs, "!")
	globIndex := 0

	l := log.WithField("localfile", localFile).WithField("globs", globList)
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
	l.WithFields(log.Fields{
		"mime_type":    kind.MIME.Type,
		"mime_subtype": kind.MIME.Subtype,
		"mime_value":   kind.MIME.Value},
	).Debug("file info")

	var ia *InnerArtifact
	for _, i := range artifact.InnerArtifacts {
		if i.LocalFile == localFile {
			ia = i
		}
	}

	// Progressively uncompress/extract files to handle nested tar.gzip
	if kind.MIME.Value == "application/gzip" {
		l.Debug("decompressing gzip")
		gz, err := gzip.NewReader(bytes.NewReader(file))
		if err != nil {
			return err
		}
		file, err = io.ReadAll(gz)
		if err != nil {
			l.WithError(err).Error("failed to read")
			return err
		}
		// update kind with the inner file
		kind, err = filetype.Match(file)
		if err != nil {
			l.WithError(err).Error("failed to match file")
			return err
		}
		l.Debugf("gzip contents is %s", kind.MIME.Value)
	}
	if kind.MIME.Value == "application/x-bzip2" {
		l.Debug("decompressing bzip2")
		bz := bzip2.NewReader(bytes.NewReader(file))
		file, err = io.ReadAll(bz)
		if err != nil {
			l.WithError(err).Error("failed to read")
			return err
		}
		// update kind with the inner file
		kind, err = filetype.Match(file)
		if err != nil {
			l.WithError(err).Error("failed to match file")
			return err
		}
		l.Debugf("bzip2 contents is %s", kind.MIME.Value)
	}
	if kind.MIME.Value == "application/x-tar" {
		l.Debug("extracting tar")
		currentGlob := globList[globIndex]
		globIndex++
		tr := tar.NewReader(bytes.NewReader(file))
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				l.WithError(err).Error("failed to read next file")
				return err
			}
			l.Debugf("tar file: %s", hdr.Name)

			match, err := path.Match(currentGlob, hdr.Name)

			if err != nil {
				l.WithError(err).Error("malformed pattern")
				return err
			}
			if match && hdr.Typeflag == tar.TypeReg {
				l.Debugf("tar file: %v", hdr)
				newInnerArtifact := false
				if ia == nil {
					newInnerArtifact = true
					ia = NewInnerArtifact()
				}
				ia.SourcePath = hdr.Name

				ia.FromGlob = currentGlob
				innerfile, err := io.ReadAll(tr)
				if err != nil {
					l.WithError(err).Error("failed to read")
					return err
				}
				csum, err := ComputeChecksum(innerfile)
				if err != nil {
					l.WithError(err).Error("failed to match checksum")
					return nil
				}
				ia.Checksums = []string{csum}
				if newInnerArtifact {
					artifact.InnerArtifacts = append(artifact.InnerArtifacts, ia)
				}
				kind, err = filetype.Match(innerfile)
				if err != nil {
					l.WithError(err).WithField("innerfile", innerfile).Error("failed to match file")
					return err
				}
				if strings.HasSuffix(localFile, "/") {
					localFile = path.Join(localFile, hdr.Name)
				}
				l.Debugf("inner file \"%s\" is %s", hdr.Name, kind.MIME.Value)

				if kind.MIME.Value == "application/x-executable" {
					ia.LocalFile = localFile
					err := installBin(innerfile, localFile)
					if err != nil {
						l.WithError(err).Error("failed to install")
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

func DownloadFile(ctx context.Context, client *http.Client, artifact *Artifact) ([]byte, error) {
	l := log.WithFields(log.Fields{"f": "DownloadFile", "url": artifact.RemoteFile})
	l.Debug("downloading file")
	if client == nil {
		client = &http.Client{}
	}
	resp, err := client.Get(artifact.RemoteFile)
	if err != nil {
		l.WithError(err).Error("failed to download")
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, errors.Errorf("%s", resp.Status)
	}

	bar := progressbar.DefaultBytes(resp.ContentLength, "downloading")
	file := new(bytes.Buffer)
	file_io := io.MultiWriter(bar, file)
	copyBytes, err := io.Copy(file_io, resp.Body)

	if err != nil {
		l.WithError(err).Error("failed to read response body")
		return nil, err
	}
	if copyBytes != resp.ContentLength {
		err = fmt.Errorf("content length mismatch")
		l.WithError(err).WithFields(log.Fields{"bytes_out": copyBytes, "ContentLength": resp.ContentLength}).Error("content length mismatch")
		return nil, err
	}

	return file.Bytes(), nil
}
