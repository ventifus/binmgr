package backend

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"hash"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/apex/log"
	"github.com/go-errors/errors"
)

const (
	ChecksumNone              = "none"
	ChecksumShasum256         = "sha256sums"
	ChecksumPerAssetSig       = "per_asset:sig"
	ChecksumPerAssetPem       = "per_asset:pem"
	ChecksumPerAssetSha256Sum = "per_asset:sha256sum"
	ChecksumPerAssetSha256    = "per_asset:sha256"
	ChecksumMultiSum          = "multisum"

	AlgorithmSha256   = "sha-256"
	AlgorithmSha384   = "sha-384"
	AlgorithmSha512   = "sha-512"
	AlgorithmSha3_256 = "sha3-256"
	AlgorithmSha3_384 = "sha3-384"
)

type ChecksumEntry struct {
	Sum  string
	Name string
}

func ChecksumTypes() []string {
	return []string{
		ChecksumNone,
		ChecksumShasum256,
		ChecksumPerAssetSig,
		ChecksumPerAssetPem,
		ChecksumPerAssetSha256Sum,
		ChecksumPerAssetSha256,
		ChecksumMultiSum,
	}
}

func ChecksumAlgorithms() []string {
	return []string{
		AlgorithmSha256,
	}
}

func getHashFuncs(types []string) map[string]hash.Hash {
	hashMap := make(map[string]hash.Hash)
	for _, ctype := range types {
		switch ctype {
		case AlgorithmSha256:
			hashMap[AlgorithmSha256] = sha256.New()
		case AlgorithmSha384:
			hashMap[AlgorithmSha384] = sha512.New384()
		case AlgorithmSha512:
			hashMap[AlgorithmSha512] = sha512.New()
		default:
			log.Debugf("not implemented: %s", ctype)
		}
	}
	return hashMap
}

func ComputeChecksums(b []byte, types []string) (map[string]string, error) {
	hashMap := getHashFuncs(types)
	results := make(map[string]string)
	for k, h := range hashMap {
		_, err := io.Copy(h, bytes.NewReader(b))
		if err != nil {
			log.WithError(err).Errorf("failed to compute checksum")
			return results, err
		}
		results[k] = hex.EncodeToString(h.Sum(nil))
	}
	return results, nil
}

func GetChecksumUrl(client *http.Client, url string) ([]ChecksumEntry, error) {
	log := log.WithField("f", "GetChecksumUrl").WithField("url", url)
	csums := make([]ChecksumEntry, 0)
	if client == nil {
		client = &http.Client{}
	}
	resp, err := client.Get(url)
	if err != nil {
		log.WithError(err).Errorf("failed to get checksum url")
		return csums, err
	}
	defer resp.Body.Close()
	log.WithField("status", resp.Status).WithField("statuscode", resp.StatusCode).Debug("get checksum url")
	if resp.StatusCode != 200 {
		return csums, errors.Errorf("%s", resp.Status)
	}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		sections := strings.SplitN(scanner.Text(), " ", 2)
		filename := strings.TrimSpace(sections[1])
		filename = strings.TrimPrefix(filename, "*") // Found one sha256sums.txt where filenames all begin with '*'
		csums = append(csums, ChecksumEntry{
			Sum:  strings.TrimSpace(sections[0]),
			Name: filename,
		})
	}
	return csums, nil
}

// GetChecksumMap retrieves a checksum file from a URL and returns a list of checksum types.
// A checksum map file contains a list of checksums, one per line.
func GetChecksumMap(client *http.Client, url string) ([]string, error) {
	log := log.WithField("f", "GetChecksumMap").WithField("url", url)
	csums := make([]string, 0)
	if client == nil {
		client = &http.Client{}
	}
	resp, err := client.Get(url)
	if err != nil {
		log.WithError(err).Errorf("failed to get checksum map")
		return csums, err
	}
	defer resp.Body.Close()
	log.WithField("status", resp.Status).WithField("statuscode", resp.StatusCode).Debug("get checksum map")
	if resp.StatusCode != 200 {
		return csums, errors.Errorf("%s", resp.Status)
	}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		line = strings.ToLower(line)
		csums = append(csums, line)
	}
	return csums, nil
}

func GetSumForFile(client *http.Client, url string, file string) (string, error) {
	l := log.WithField("f", "GetSumForFile").WithField("url", url).WithField("file", file)
	if client == nil {
		client = &http.Client{}
	}
	resp, err := client.Get(url)
	if err != nil {
		l.WithError(err).Errorf("failed to get checksum for file")
		return "", err
	}
	defer resp.Body.Close()
	l.WithField("status", resp.Status).WithField("statuscode", resp.StatusCode).Info("get checksum for file")
	if resp.StatusCode != 200 {
		return "", errors.Errorf("%s", resp.Status)
	}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		sections := strings.SplitN(scanner.Text(), " ", 2)
		sections[0] = strings.TrimSpace(sections[0])
		filename := sections[1]
		filename = strings.TrimSpace(filename)
		filename = strings.TrimPrefix(filename, "*")
		filename = strings.TrimPrefix(filename, "./")
		l.WithFields(log.Fields{"orig": sections[1], "file": filename}).Debug("checksum entry")
		if filename == file {
			return sections[0], nil
		}
		l.WithField("filename", filename).Debug("skipping file")
	}
	return "", errors.Errorf("no checksum found for file: %s", file)
}

func GetMultiSumForFile(client *http.Client, csumMap []string, url string, file string) (map[string]string, error) {
	l := log.WithField("f", "GetMultiSumForFile").WithFields(log.Fields{"url": url, "file": file, "checksums": csumMap})
	results := make(map[string]string)
	if client == nil {
		client = &http.Client{}
	}
	resp, err := client.Get(url)
	if err != nil {
		log.WithError(err).Errorf("failed to get multisum for file")
		return results, err
	}
	defer resp.Body.Close()
	l.WithField("status", resp.Status).WithField("statuscode", resp.StatusCode).Info("get multisum for file")
	if resp.StatusCode != 200 {
		return results, errors.Errorf("%s", resp.Status)
	}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		sections := strings.Split(scanner.Text(), "  ")
		filename := sections[0]
		filename = strings.TrimSpace(filename)
		filename = strings.TrimPrefix(filename, "*")
		filename = strings.TrimPrefix(filename, "./")
		l.WithFields(log.Fields{"orig": sections[0], "file": filename}).Debug("checksum entry")
		if filename == file {
			l.Debugf("%s: %v", filename, sections)
			for i, ctype := range csumMap {
				log.WithField("file", file).Debugf("checksum[%d/%s] = %s", i, ctype, sections[i+1])
				results[ctype] = sections[i+1]
			}
			return results, nil
		}
		l.WithField("filename", filename).Debug("skipping file")
	}
	return results, errors.Errorf("no checksum found for file: %s", file)
}

func VerifyBytes(b []byte, checksum map[string]string) error {
	log := log.WithField("f", "VerifyBytes")
	algos := make([]string, len(checksum))
	i := 0
	for k, _ := range checksum {
		algos[i] = k
		i++
	}
	csums, err := ComputeChecksums(b, algos)
	if err != nil {
		return err
	}

	log.Debugf("desired checksum: %v", csums)
	for k, v := range checksum {
		if v == "" || csums[k] == "" {
			log.Debugf("skipping empty checksum for %s", k)
			continue
		}
		l := log.WithField("algorithm", k)
		if csums[k] != v {
			l.Debugf("checksum mismatch: %s != %s", v, csums[k])
			return errors.Errorf("checksum mismatch for %s: expected %s, got %s", k, v, csums[k])
		}
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
			log.Info("verifying local file")
			err = VerifyBytes(lb, artifact.Checksums)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
