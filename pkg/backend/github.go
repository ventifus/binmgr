package backend

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/apex/log"
	"github.com/go-errors/errors"
	"github.com/google/go-github/v55/github"
)

type GithubAsset struct {
	Name      string
	Version   string
	Url       string
	Checksums []string
}

type GithubRepo struct {
	Manifest      *BinmgrManifest
	release       *github.RepositoryRelease
	log           *log.Entry
	client        *github.Client
	owner         string
	repo          string
	checksumfiles []*github.ReleaseAsset
}

func InstallGithub(ctx context.Context, githubUrl *url.URL, fileGlob string, outFile string) error {
	if githubUrl.Host != "github.com" {
		return fmt.Errorf("this type is only valid for github.com")
	}
	githubPath := strings.Split(githubUrl.Path, "/")
	owner := githubPath[1]
	repo := githubPath[2]

	if !path.IsAbs(outFile) {
		outFile = path.Join(os.Getenv("HOME"), ".local/bin/", outFile)
	}

	gh := NewGithubRepo(owner, repo)
	err := gh.GetRelease(ctx, githubPath)
	if err != nil {
		return err
	}
	err = gh.SelectAssetByGlob(fileGlob)
	if err != nil {
		return err
	}

	for _, artifact := range gh.Manifest.Artifacts {
		err = VerifyLocalFile(artifact)
		if err == nil {
			log.Info("local file exists and matches checksum; nothing to do")
			continue
		} else if !os.IsNotExist(err) {
			return err
		}

		f, err := DownloadFile(ctx, nil, artifact)
		if err != nil {
			log.WithError(err).Error("failed to read response data")
			return err
		}

		fmt.Printf("Installing %s from %s\n", path.Base(outFile), artifact.RemoteFile)

		err = InstallFile(artifact, f, outFile, artifact.FromGlob)
		if err != nil {
			return err
		}
	}
	return gh.Manifest.SaveManifest()
}

func UpdateGithub(ctx context.Context, m *BinmgrManifest) error {
	log := log.WithField("manifest", m.Name)
	currentVersion := m.CurrentVersion
	fmt.Printf("Package %s %s\n", m.Name, currentVersion)
	repo := NewGithubRepoFromManifest(m)
	err := repo.GetRelease(ctx, nil)
	if err != nil {
		return err
	}
	if currentVersion == repo.Manifest.CurrentVersion {
		log.WithField("version", repo.Manifest.CurrentVersion).Debug("no update found")
		fmt.Println("  no update needed")
		return nil
	}
	fmt.Printf("  upgrade %s -> %s\n", currentVersion, repo.Manifest.CurrentVersion)
	log.WithField("manifest", repo.Manifest.String()).Info("received manifest")
	updates := false
	for i, artifact := range repo.Manifest.Artifacts {
		log.WithField("artifact", artifact).Debug("processing artifact")
		newArtifact, err := repo.UpdateArtifact(artifact)
		if err != nil {
			log.WithError(err).Error("failed to update artifact")
			continue
		}
		newArtifact.LocalFile = artifact.LocalFile
		newArtifact.InnerArtifacts = artifact.InnerArtifacts
		newArtifact.Installed = artifact.Installed
		if newArtifact.RemoteFile == artifact.RemoteFile {
			log.Debug("no update needed")
			continue
		}
		repo.Manifest.Artifacts[i] = newArtifact

		f, err := DownloadFile(ctx, nil, newArtifact)
		if err != nil {
			log.WithError(err).Error("failed to read response data")
			return err
		}

		if newArtifact.Installed {
			err = InstallFile(newArtifact, f, newArtifact.LocalFile, newArtifact.FromGlob)
			if err != nil {
				return err
			}
			updates = true
		} else {
			for _, ia := range newArtifact.InnerArtifacts {
				if ia.Installed {
					err = InstallFile(newArtifact, f, ia.LocalFile, ia.FromGlob)
					if err != nil {
						return err
					}
					updates = true
				}
			}
		}
	}
	if updates {
		return repo.Manifest.SaveManifest()
	}
	return nil
}

func GithubStatus(ctx context.Context, m *BinmgrManifest) error {
	log := log.WithField("manifest", m.Name)
	currentVersion := m.CurrentVersion
	fmt.Printf("Package %s %s\n", m.Name, currentVersion)
	repo := NewGithubRepoFromManifest(m)
	err := repo.GetRelease(ctx, nil)
	if err != nil {
		return err
	}
	if currentVersion == repo.Manifest.CurrentVersion {
		log.WithField("version", repo.Manifest.CurrentVersion).Debug("no update found")
		fmt.Println("  no update needed")
		// pp.Println(m)
		// pp.Println(repo.Manifest)
		return nil
	}
	fmt.Printf("  upgrade %s -> %s\n", currentVersion, repo.Manifest.CurrentVersion)
	return nil
}

func getAssetByGlobOrPartial(release *github.RepositoryRelease, nameGlob string) (*github.ReleaseAsset, error) {
	for _, asset := range release.Assets {
		t, err := filepath.Match(nameGlob, asset.GetName())
		if err != nil {
			return nil, err
		}
		if t {
			return asset, nil
		}
	}
	return nil, errors.Errorf("asset found matching %s", nameGlob)
}

func findShaSums(release *github.RepositoryRelease) []*github.ReleaseAsset {
	shasums := make([]*github.ReleaseAsset, 0)
	for _, asset := range release.Assets {
		assetName := asset.GetName()
		if !strings.HasSuffix(assetName, ".pem") && !strings.HasSuffix(assetName, ".sig") {
			if (strings.Contains(assetName, "sha") && strings.Contains(assetName, "sum")) ||
				strings.Contains(assetName, "checksum") {
				log.WithField("url", *asset.BrowserDownloadURL).Debug("using checksum file")
				shasums = append(shasums, asset)
			}
		}
	}
	if len(shasums) == 0 {
		log.Debug("no checksums selected")
	}
	return shasums
}

func NewGithubRepo(owner string, repo string) *GithubRepo {
	r := GithubRepo{
		Manifest: &BinmgrManifest{
			Type: "github",
			Properties: map[string]string{
				"owner": owner,
				"repo":  repo,
			},
		},
		log:    log.WithField("owner", owner).WithField("repo", repo),
		client: github.NewClient(nil),
		owner:  owner,
		repo:   repo,
	}
	return &r
}

func NewGithubRepoFromManifest(manifest *BinmgrManifest) *GithubRepo {
	owner := manifest.Properties["owner"]
	repo := manifest.Properties["repo"]
	r := GithubRepo{
		Manifest: manifest,
		log:      log.WithField("owner", owner).WithField("repo", repo),
		client:   github.NewClient(nil),
		owner:    owner,
		repo:     repo,
	}
	return &r
}

func (g *GithubRepo) GetRelease(ctx context.Context, u []string) error {
	log := g.log.WithField("manifest", g.Manifest.Name).WithField("url", u)
	var release *github.RepositoryRelease
	var resp *github.Response
	var err error
	if len(u) < 4 {
		log.WithField("release", "latest").Debug("getting latest release")
		release, resp, err = g.client.Repositories.GetLatestRelease(ctx, g.owner, g.repo)
	} else if u[4] == "tag" {
		log.WithField("release", u[5]).Debug("getting release tag")
		release, resp, err = g.client.Repositories.GetReleaseByTag(ctx, g.owner, g.repo, u[5])
	} else if u[4] == "id" {
		var rel int64
		rel, err = strconv.ParseInt(u[5], 10, 64)
		if err != nil {
			return err
		}
		log.WithField("release", rel).Debug("getting release id")
		release, resp, err = g.client.Repositories.GetRelease(ctx, g.owner, g.repo, rel)
	} else {
		log.WithField("u[3]", u[4]).Error("unsupported release type")
	}
	if err != nil {
		return err
	} else if resp.StatusCode != 200 {
		log.WithField("status", resp.Status).WithField("statuscode", resp.StatusCode).Error("failed to get github release")
		return errors.Errorf("github.com/%s/%s: %s", g.owner, g.repo, resp.Status)
	}
	log.WithField("status", resp.Status).WithField("statuscode", resp.StatusCode).Debug("get github release")
	g.release = release
	g.Manifest.Name = fmt.Sprintf("github.com/%s/%s", g.owner, g.repo)
	g.Manifest.ManifestFileName = fmt.Sprintf("github.com_%s_%s", g.owner, g.repo)
	g.Manifest.CurrentRemoteUrl = release.GetURL()
	g.Manifest.CurrentVersion = release.GetTagName()
	g.Manifest.LatestRemoteUrl = fmt.Sprintf("https://github.com/%s/%s", g.owner, g.repo)
	g.Manifest.Properties["owner"] = g.owner
	g.Manifest.Properties["repo"] = g.repo
	g.checksumfiles = findShaSums(g.release)
	return nil
}

func (g *GithubRepo) newArtifactFromAsset(asset *github.ReleaseAsset) (*Artifact, error) {
	checksums := make([]string, 0)
	for _, csum := range g.checksumfiles {
		checksum, err := GetSumForFile(nil, csum.GetBrowserDownloadURL(), asset.GetName())
		if err != nil {
			return nil, err
		}
		checksums = append(checksums, checksum)
	}
	artifact := NewArtifact()
	artifact.AssetUrl = *asset.URL
	artifact.RemoteFile = asset.GetBrowserDownloadURL()
	artifact.Checksums = checksums
	artifact.Installed = false
	return artifact, nil
}

func (g *GithubRepo) SelectAssetByGlob(glob string) error {
	asset, err := getAssetByGlobOrPartial(g.release, glob)
	if err != nil {
		return err
	}
	artifact, err := g.newArtifactFromAsset(asset)
	if err != nil {
		return err
	}
	artifact.FromGlob = glob
	g.Manifest.Artifacts = append(g.Manifest.Artifacts, artifact)
	return nil
}

func (g *GithubRepo) UpdateArtifact(a *Artifact) (*Artifact, error) {
	asset, err := getAssetByGlobOrPartial(g.release, a.FromGlob)
	if err != nil {
		return nil, err
	}
	artifact, err := g.newArtifactFromAsset(asset)
	artifact.FromGlob = a.FromGlob
	return artifact, err
}

func GetGithubManifest(ctx context.Context, owner string, repo string, remoteNameGlob string, localName string) (*BinmgrManifest, error) {
	log := log.WithField("f", "GetGithubManifest").WithField("owner", owner).WithField("repo", repo)
	manifest := &BinmgrManifest{Type: "github", Properties: make(map[string]string)}
	client := github.NewClient(nil)
	release, resp, err := client.Repositories.GetLatestRelease(ctx, owner, repo)
	log = log.WithField("url", resp.Request.URL)
	if err != nil {
		log.WithError(err).Errorf("failed to get github release")
		return nil, err
	}
	if resp.StatusCode != 200 {
		log.WithField("status", resp.Status).WithField("statuscode", resp.StatusCode).Errorf("failed to get github release")
		return nil, errors.Errorf("github.com/%s/%s: %s", owner, repo, resp.Status)
	} else {
		log.WithField("status", resp.Status).WithField("statuscode", resp.StatusCode).Debug("get github release")
	}
	manifest.Name = fmt.Sprintf("github.com_%s_%s", owner, repo)
	manifest.CurrentRemoteUrl = release.GetURL()
	manifest.CurrentVersion = release.GetTagName()
	manifest.LatestRemoteUrl = fmt.Sprintf("https://github.com/%s/%s", owner, repo)
	manifest.Properties["owner"] = owner
	manifest.Properties["repo"] = repo

	asset, err := getAssetByGlobOrPartial(release, remoteNameGlob)
	if err != nil {
		return nil, err
	}
	checksums := make([]string, 0)
	if asset != nil {
		shasums := findShaSums(release)
		for _, csum := range shasums {
			manifest.ChecksumFile = csum.GetBrowserDownloadURL()
			checksum, err := GetSumForFile(nil, csum.GetBrowserDownloadURL(), asset.GetName())
			if err != nil {
				return nil, err
			}
			checksums = append(checksums, checksum)
		}
		manifest.Artifacts = append(manifest.Artifacts, &Artifact{
			LocalFile:  localName,
			AssetUrl:   *asset.URL,
			RemoteFile: asset.GetBrowserDownloadURL(),
			Checksums:  checksums,
			Installed:  false,
			FromGlob:   remoteNameGlob,
		})
	}
	return manifest, nil
}

// func GetLatestGithubAsset(ctx context.Context, owner string, repo string, file string) (*GithubAsset, error) {
// 	log := log.WithField("f", "GetLatestGithubAsset").WithField("owner", owner).WithField("repo", repo)
// 	log.Infof("getting latest github asset")
// 	release, err := getLatestRelease(ctx, owner, repo)
// 	if err != nil {
// 		return nil, err
// 	}
// 	asset, err := getAssetByGlobOrPartial(release, file)
// 	if err != nil {
// 		return nil, err
// 	}
// 	checksums := make([]string, 0)
// 	if asset != nil {
// 		shasums := findShaSums(release)
// 		for _, csum := range shasums {
// 			checksum, err := GetSumForFile(nil, csum.GetBrowserDownloadURL(), asset.GetName())
// 			if err != nil {
// 				return nil, err
// 			}
// 			checksums = append(checksums, checksum)
// 		}
// 		return &GithubAsset{
// 			Name:      asset.GetName(),
// 			Version:   release.GetTagName(),
// 			Url:       asset.GetBrowserDownloadURL(),
// 			Checksums: checksums,
// 		}, nil
// 	}
// 	return nil, errors.Errorf("asset %s not found", file)
// }
