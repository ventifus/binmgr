package backend

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/apex/log"
	"github.com/go-errors/errors"
)

// Installs kubernetes components

const kubeVersionUrl = "https://dl.k8s.io/release/stable.txt"

func getUrlAsString(client *http.Client, u string) (string, error) {
	log := log.WithField("url", u)
	ret := ""
	if client == nil {
		client = &http.Client{}
	}
	resp, err := client.Get(u)
	if err != nil {
		log.WithError(err).Errorf("failed to get url")
		return ret, err
	}
	defer resp.Body.Close()
	log.WithField("status", resp.Status).WithField("statuscode", resp.StatusCode).Debug("get file")
	if resp.StatusCode != 200 {
		return ret, errors.Errorf("%s", resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		log.WithError(err).Errorf("failed to read url")
		return ret, err
	}
	ret = string(b)
	return ret, nil
}

func InstallKubeFile(ctx context.Context, u *url.URL, fileGlob string, outFile string) error {
	pathComponents := strings.Split(u.Path, "/")

	m := NewBinmgrManifest()
	m.Type = "kubeurl"
	if outFile != "" && !path.IsAbs(outFile) {
		outFile = path.Join(os.Getenv("HOME"), ".local/bin/", outFile)
	}
	m.Name = fmt.Sprintf("dl.k8s.io/.../%s", strings.Join(pathComponents[2:], "/"))
	m.CurrentRemoteUrl = kubeVersionUrl
	m.ManifestFileName = fmt.Sprintf("kubeurl_%s", strings.Join(pathComponents[2:], "_"))

	kubeVersion, err := getUrlAsString(nil, kubeVersionUrl)
	if err != nil {
		return err
	}
	m.CurrentVersion = kubeVersion

	pathComponents[1] = kubeVersion
	u.Path = strings.Join(pathComponents, "/")
	m.LatestRemoteUrl = u.String()

	a := NewArtifact()
	a.RemoteFile = u.String()
	if outFile == "" {
		a.LocalFile = pathComponents[len(pathComponents)-1]
		outFile = path.Join(os.Getenv("HOME"), ".local/bin/", a.LocalFile)
	}

	m.ChecksumFile = fmt.Sprintf("%s.sha256", a.RemoteFile)
	shasum, err := getUrlAsString(nil, m.ChecksumFile)
	if err != nil {
		log.WithError(err).Errorf("failed to get checksum")
		return err
	}
	a.Checksums = []string{shasum}

	m.Artifacts = append(m.Artifacts, a)
	fmt.Printf("Installing %s from %s\n", path.Base(outFile), a.RemoteFile)

	file, err := DownloadFile(ctx, nil, a)
	if err != nil {
		log.WithError(err).Errorf("failed to download file")
		return err
	}
	err = InstallFile(a, file, outFile, a.FromGlob)
	if err != nil {
		return err
	}

	return m.SaveManifest()
}

func UpdateKubeUrl(ctx context.Context, m *BinmgrManifest) error {
	kubeVersion, err := getUrlAsString(nil, kubeVersionUrl)
	if err != nil {
		return err
	}
	updates := false
	fmt.Printf("Package %s %s\n", m.Name, m.CurrentVersion)
	if m.CurrentVersion != kubeVersion {
		updates = true
		for _, a := range m.Artifacts {
			u, err := url.Parse(a.RemoteFile)
			if err != nil {
				log.WithError(err).WithField("RemoteFile", a.RemoteFile).Error("failed to parse RemoteFile url")
				continue
			}
			pathComponents := strings.Split(u.Path, "/")
			pathComponents[1] = kubeVersion
			u.Path = strings.Join(pathComponents, "/")
			remoteFile := u.String()
			fmt.Printf("  upgrade %s -> %s\n", path.Base(a.RemoteFile), path.Base(remoteFile))
			a.RemoteFile = remoteFile
			m.ChecksumFile = fmt.Sprintf("%s.sha256", a.RemoteFile)
			shasum, err := getUrlAsString(nil, m.ChecksumFile)
			if err != nil {
				log.WithError(err).Errorf("failed to get checksum")
				return err
			}
			a.Checksums = []string{shasum}
			file, err := DownloadFile(ctx, nil, a)
			if err != nil {
				log.WithError(err).Errorf("failed to download file")
				return err
			}
			fmt.Printf("    - %s\n", a.LocalFile)
			err = InstallFile(a, file, a.LocalFile, a.FromGlob)
			if err != nil {
				return err
			}
		}
	}
	if updates {
		return m.SaveManifest()
	}
	fmt.Println("  no update needed")
	return nil
}

func KubeUrlStatus(ctx context.Context, m *BinmgrManifest) error {
	kubeVersion, err := getUrlAsString(nil, kubeVersionUrl)
	if err != nil {
		return err
	}

	fmt.Printf("Package %s %s\n", m.Name, kubeVersion)
	if kubeVersion != m.CurrentVersion {
		fmt.Printf("  upgrade %s -> %s", m.CurrentVersion, kubeVersion)
		return nil
	}

	fmt.Println("  no update needed")
	return nil
}
