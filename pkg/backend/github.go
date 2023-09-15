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

func InstallGithub(ctx context.Context, u string, fileGlob string, outFile string) error {
	if strings.HasPrefix(u, "github.com") {
		u = fmt.Sprintf("https://%s", u)
	}
	githubUrl, err := url.Parse(u)
	if err != nil {
		return err
	}
	githubPath := strings.Split(githubUrl.Path, "/")
	owner := githubPath[1]
	repo := githubPath[2]

	if !path.IsAbs(outFile) {
		outFile = path.Join(os.Getenv("HOME"), ".local/bin/", outFile)
	}

	gh := NewGithubRepo(owner, repo)
	err = gh.GetLatestRelease(ctx)
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
			return err
		}
		if err != nil {
			log.WithError(err).Error("failed to read response data")
			return err
		}

		err = InstallFile(artifact, f, outFile)
		if err != nil {
			return err
		}
	}
	return gh.Manifest.SaveManifest()
}

func UpdateGithub(ctx context.Context, m *BinmgrManifest) error {
	log := log.WithField("manifest", m.Name)
	fmt.Printf("Package %s %s\n", m.Name, m.CurrentVersion)
	repo := NewGithubRepoFromManifest(m)
	err := repo.GetLatestRelease(ctx)
	if err != nil {
		return err
	}
	if m.CurrentVersion == repo.Manifest.CurrentVersion {
		log.WithField("version", repo.Manifest.CurrentVersion).Debug("no update found")
		fmt.Println("  no update needed")
		return nil
	}
	fmt.Printf("  upgrade %s -> %s\n", m.CurrentVersion, repo.Manifest.CurrentVersion)
	repo.Manifest.Artifacts = m.Artifacts

	log.WithField("manifest", repo.Manifest.String()).Info("received manifest")
	updates := false
	for i, artifact := range repo.Manifest.Artifacts {
		if !artifact.Installed {
			continue
		}
		newArtifact, err := repo.UpdateArtifact(artifact)
		newArtifact.LocalFile = artifact.RemoteFile
		newArtifact.InnerArtifacts = artifact.InnerArtifacts
		if err != nil {
			log.WithError(err).Error("failed to update artifact")
			continue
		}
		if newArtifact.RemoteFile == artifact.RemoteFile {
			log.Debug("no update needed")
			continue
		}
		repo.Manifest.Artifacts[i] = newArtifact
		err = VerifyLocalFile(newArtifact)
		if err == nil {
			log.Infof("local file exists and matches checksum; nothing to do")
			fmt.Printf("  - %s no update needed\n", newArtifact.LocalFile)
			newArtifact.Installed = true
			continue
		} else if !os.IsNotExist(err) {
			return err
		}

		f, err := DownloadFile(ctx, nil, newArtifact)
		if err != nil {
			log.WithError(err).Error("failed to read response data")
			return err
		}

		err = InstallFile(newArtifact, f, newArtifact.LocalFile)
		if err != nil {
			return err
		}
		updates = true
		newArtifact.Installed = true
	}
	if updates {
		return repo.Manifest.SaveManifest()
	}
	return nil
}

func GithubStatus(ctx context.Context, m *BinmgrManifest) error {
	log := log.WithField("manifest", m.Name)
	fmt.Printf("Package %s %s\n", m.Name, m.CurrentVersion)
	repo := NewGithubRepoFromManifest(m)
	err := repo.GetLatestRelease(ctx)
	if err != nil {
		return err
	}
	if m.CurrentVersion == repo.Manifest.CurrentVersion {
		log.WithField("version", repo.Manifest.CurrentVersion).Debug("no update found")
		fmt.Println("  no update needed")
		return nil
	}
	fmt.Printf("  upgrade %s -> %s\n", m.CurrentVersion, repo.Manifest.CurrentVersion)
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
	return nil, fmt.Errorf("no matching asset found")
}

func findShaSums(release *github.RepositoryRelease) []*github.ReleaseAsset {
	shasums := make([]*github.ReleaseAsset, 0)
	for _, asset := range release.Assets {
		assetName := asset.GetName()
		if (strings.Contains(assetName, "sha") &&
			strings.Contains(assetName, "sum")) ||
			strings.Contains(assetName, "checksum") {
			shasums = append(shasums, asset)
		}
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

func (g *GithubRepo) GetLatestRelease(ctx context.Context) error {
	log := g.log.WithField("manifest", g.Manifest.Name)
	release, resp, err := g.client.Repositories.GetLatestRelease(ctx, g.owner, g.repo)
	if err != nil {
		return err
	} else if resp.StatusCode != 200 {
		log.WithField("status", resp.Status).WithField("statuscode", resp.StatusCode).Error("failed to get github release")
		return fmt.Errorf("github.com/%s/%s: %s", g.owner, g.repo, resp.Status)
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
	artifact.FromGlob = glob
	if err != nil {
		return err
	}
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
		return nil, fmt.Errorf("github.com/%s/%s: %s", owner, repo, resp.Status)
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
// 	return nil, fmt.Errorf("asset %s not found", file)
// }
