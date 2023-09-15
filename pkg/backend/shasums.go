package backend

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/apex/log"
)

type ChecksumEntry struct {
	Sum  string
	Name string
}

func ComputeChecksum(b []byte) (string, error) {
	csum := sha256.New()
	_, err := io.Copy(csum, bytes.NewReader(b))
	if err != nil {
		log.WithError(err).Errorf("failed to compute checksum")
		return "", err
	}
	return hex.EncodeToString(csum.Sum(nil)), nil
}

func GetChecksumUrl(client *http.Client, url string) ([]ChecksumEntry, error) {
	log := log.WithField("f", "GetChecksumUrl").WithField("url", url)
	csums := make([]ChecksumEntry, 0)
	if client == nil {
		client = &http.Client{}
	}
	resp, err := client.Get(url)
	if err != nil {
		log.WithError(err).Errorf("failed to get checksum file")
		return csums, err
	}
	defer resp.Body.Close()
	log.WithField("status", resp.Status).WithField("statuscode", resp.StatusCode).Debug("get checksum file")
	if resp.StatusCode != 200 {
		return csums, fmt.Errorf("%s", resp.Status)
	}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		sections := strings.SplitN(scanner.Text(), " ", 2)
		csums = append(csums, ChecksumEntry{
			Sum:  strings.TrimSpace(sections[0]),
			Name: strings.TrimSpace(sections[1]),
		})
	}
	return csums, nil
}

func GetSumForFile(client *http.Client, url string, file string) (string, error) {
	log := log.WithField("f", "GetSumForFile").WithField("url", url).WithField("file", file)
	if client == nil {
		client = &http.Client{}
	}
	resp, err := client.Get(url)
	if err != nil {
		log.WithError(err).Errorf("failed to get checksum file")
		return "", err
	}
	defer resp.Body.Close()
	log.WithField("status", resp.Status).WithField("statuscode", resp.StatusCode).Info("get checksum file")
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("%s", resp.Status)
	}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		sections := strings.SplitN(scanner.Text(), " ", 2)
		sections[0] = strings.TrimSpace(sections[0])
		sections[1] = strings.TrimSpace(sections[1])
		if sections[1] == file {
			return sections[0], nil
		}
	}
	return "", fmt.Errorf("no checksum found for file: %s", file)
}

func VerifyBytes(b []byte, checksum string) error {
	log := log.WithField("f", "VerifyBytes")
	log.Debugf("desired checksum: \"%s\"", checksum)
	csum := sha256.New()
	_, err := io.Copy(csum, bytes.NewReader(b))
	if err != nil {
		log.WithError(err).Errorf("failed to compute checksum")
		return err
	}
	h := hex.EncodeToString(csum.Sum(nil))
	log.Debugf("computed checksum: \"%s\"", h)
	if h != checksum {
		return fmt.Errorf("checksum does not match")
	}
	return nil
}

func VerifyLocalFile(artifact *Artifact) error {
	log := log.WithField("f", "VerifyLocalFile").WithField("file", artifact.LocalFile)
	fi, err := os.Stat(artifact.LocalFile)
	if os.IsNotExist(err) {
		return err
	} else if err != nil {
		log.WithError(err).Info("failed to stat file")
		return err
	}
	log.WithField("mode", fi.Mode()).Info("stat")
	if fi != nil && fi.Mode().IsRegular() {
		log.Info("local file exists and is regular")
		l, err := os.Open(artifact.LocalFile)
		if err != nil {
			log.WithError(err).Errorf("failed to open file")
			return err
		} else {
			lb, err := io.ReadAll(l)
			if err != nil {
				log.WithError(err).Error("failed to read file")
				return err
			}
			for _, csum := range artifact.Checksums {
				log.Info("verifying local file")
				err = VerifyBytes(lb, csum)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}
