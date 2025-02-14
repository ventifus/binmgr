package backend

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/apex/log"
)

// Installs files by starting with a fixed url to a shasum manifest that looks like
//    f24ea6a5d24b0cf44a215544f4df21972872971ea2387ca966a20aeed38f2cd8  ccoctl-linux-4.14.1.tar.gz
//	  f17c71622d9a07ef148e23f4eb400af14cb34c2a6bba3b9d9fed53030420f70e  openshift-client-linux-4.14.1.tar.gz
//    75228b51ffdeb6b85dcbb63e1532ea05c1b3c43308dd8b5c3598a0a73f4515ab  openshift-client-linux-arm64-4.14.1.tar.gz

func InstallShasumUrl(ctx context.Context, u *url.URL, fileGlob string, outFile string) error {
	csums, err := GetChecksumUrl(nil, u.String())
	if err != nil {
		log.WithError(err).Errorf("failed to get checksum file")
		return err
	}
	m := NewBinmgrManifest()
	m.Type = "shasumurl"
	if !path.IsAbs(outFile) {
		outFile = path.Join(os.Getenv("HOME"), ".local/bin/", outFile)
	}
	m.Name = u.String()
	m.ManifestFileName = fmt.Sprintf("shasumurl_%s", strings.ReplaceAll(u.String(), "/", "_"))
	m.LatestRemoteUrl = u.String()

	for _, f := range csums {
		t, err := filepath.Match(fileGlob, f.Name)
		if err != nil {
			log.WithField("fileGlob", fileGlob).WithError(err).Errorf("failed to match fileglob")
			return err
		}
		if t {
			a := NewArtifact()
			uRel, err := url.Parse(f.Name)
			if err != nil {
				return err
			}

			a.RemoteFile = u.ResolveReference(uRel).String()
			a.FromGlob = fileGlob
			m.Artifacts = append(m.Artifacts, a)
			fmt.Printf("Installing %s from %s\n", path.Base(outFile), a.RemoteFile)

			file, err := DownloadFile(ctx, nil, a)
			if err != nil {
				log.WithError(err).Error("failed to read response data")
				return err
			}
			err = InstallFile(a, file, outFile)
			if err != nil {
				return err
			}
		}
	}
	return m.SaveManifest()
}

func UpdateShasumUrl(ctx context.Context, m *BinmgrManifest) error {
	csums, err := GetChecksumUrl(nil, m.LatestRemoteUrl)
	if err != nil {
		return err
	}
	fmt.Printf("Package %s\n", m.Name)
	updates := false
	for _, a := range m.Artifacts {
		for _, c := range csums {
			t, err := path.Match(a.FromGlob, c.Name)
			if err != nil {
				return err
			}
			if t {
				uu, err := url.Parse(m.LatestRemoteUrl)
				if err != nil {
					return err
				}
				uRel, err := url.Parse(c.Name)
				if err != nil {
					return err
				}
				remoteFile := uu.ResolveReference(uRel).String()
				if a.RemoteFile != remoteFile {
					updates = true
					fmt.Printf("  upgrade %s -> %s\n", path.Base(a.RemoteFile), path.Base(remoteFile))
					a.RemoteFile = remoteFile
					file, err := DownloadFile(ctx, nil, a)
					if err != nil {
						log.WithError(err).Error("failed to read response data")
						return err
					}
					for _, ia := range a.InnerArtifacts {
						fmt.Printf("    - %s\n", ia.LocalFile)
						err = InstallFile(a, file, ia.LocalFile)
						if err != nil {
							return err
						}
					}
					if a.LocalFile != "" {
						fmt.Printf("    - %s\n", a.LocalFile)
						err = InstallFile(a, file, a.LocalFile)
						if err != nil {
							return err
						}
					}
				}
			}
		}
	}
	if updates {
		return m.SaveManifest()
	}
	fmt.Println("  no update needed")
	return nil
}

func ShasumUrlStatus(ctx context.Context, m *BinmgrManifest) error {
	csums, err := GetChecksumUrl(nil, m.LatestRemoteUrl)
	if err != nil {
		return err
	}
	fmt.Printf("Package %s\n", m.Name)
	updates := false
	for _, a := range m.Artifacts {
		for _, c := range csums {
			t, err := path.Match(a.FromGlob, c.Name)
			if err != nil {
				return err
			}
			if t {
				uu, err := url.Parse(m.LatestRemoteUrl)
				if err != nil {
					return err
				}
				uRel, err := url.Parse(c.Name)
				if err != nil {
					return err
				}
				remoteFile := uu.ResolveReference(uRel).String()
				if a.RemoteFile != remoteFile {
					fmt.Printf("  upgrade %s -> %s\n", path.Base(a.RemoteFile), path.Base(remoteFile))
					updates = true
				}
			}
		}
	}
	if !updates {
		fmt.Println("  no update needed")
	}
	return nil
}
