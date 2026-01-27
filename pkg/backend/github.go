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
	"github.com/k0kubun/pp/v3"
	"go.yaml.in/yaml/v3"
)

type GithubAsset struct {
	Name      string
	Version   string
	Url       string
	Checksums []string
}

type GithubRepo struct {
	Manifest *BinmgrManifest
	release  *github.RepositoryRelease
	log      *log.Entry
	client   *github.Client
	owner    string
	repo     string
}

type GhConfig struct {
	Hosts map[string]GhHostConfig `yaml:",inline"`
}

type GhHostConfig struct {
	User        string                  `yaml:"user"`
	OauthToken  string                  `yaml:"oauth_token"`
	GitProtocol string                  `yaml:"git_protocol"`
	Users       map[string]GhUserConfig `yaml:"users"`
}

type GhUserConfig struct {
	OauthToken string `yaml:"oauth_token"`
}

func expandVariables(release *github.RepositoryRelease, s string) string {
	tag := release.GetTagName()
	version := strings.TrimPrefix(tag, "v")
	s = strings.ReplaceAll(s, "${TAG}", tag)
	s = strings.ReplaceAll(s, "${VERSION}", version)
	return s
}

func getGithubClientFromGhConfig(f *os.File) (*github.Client, error) {
	ghConfig, err := parseGhConfig(f)
	if err != nil {
		return nil, err
	}
	hostConfig, ok := ghConfig.Hosts["github.com"]
	if !ok {
		pp.Println(ghConfig)
		return nil, errors.Errorf("no github.com host found in gh config")
	}
	token := hostConfig.OauthToken
	if token == "" {
		return nil, errors.Errorf("no oauth_token found in gh config for github.com")
	}
	ts := github.BasicAuthTransport{
		Username: "oauth2",
		Password: token,
	}
	client := github.NewClient(ts.Client())
	return client, nil
}

func parseGhConfig(f *os.File) (*GhConfig, error) {
	ghConfig := &GhConfig{}
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	data := make([]byte, stat.Size())
	_, err = f.Read(data)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(data, ghConfig)
	if err != nil {
		return nil, err
	}
	return ghConfig, nil
}

func newGithubClient() *github.Client {
	// load oauth token from ~/.config/gh/hosts.yml if available
	ctx := context.TODO()
	var client *github.Client
	f, err := os.Open(path.Join(os.Getenv("HOME"), ".config/gh/hosts.yml"))
	if err != nil {
		log.WithError(err).Warn("could not open gh config file; using unauthenticated github client")
		client = github.NewClient(nil)
	} else {
		client, err = getGithubClientFromGhConfig(f)
		f.Close()
		if err != nil {
			log.WithError(err).Error("failed to create github client from gh config")
		}
	}
	if client == nil {
		log.Debug("Using anonymous github access")
		client = github.NewClient(nil)
	}

	u, _, err := client.Users.Get(ctx, "")
	if u != nil {
		log.Debugf("using github user: %s", u.GetLogin())
	}

	rl, _, err := client.RateLimits(ctx)
	if err == nil {
		log.WithField("rate", rl.Core.Remaining).WithField("reset", rl.Core.Reset.Time).Debug("github rate limit")
	} else {
		log.WithError(err).Error("failed to get github rate limit")
	}
	client.APIMeta(ctx)
	return client
}

func InstallGithub(ctx context.Context, githubUrl *url.URL, fileGlob string, outFile string, checksumType string) error {
	if githubUrl.Host != "github.com" {
		return fmt.Errorf("this type is only valid for github.com")
	}
	log := log.WithField("url", githubUrl.String())

	githubPath := strings.Split(githubUrl.Path, "/")
	owner := githubPath[1]
	repo := githubPath[2]

	if outFile == "" {
		outFile = path.Join(os.Getenv("HOME"), ".local/bin") + "/"
		log.Debugf("outFile not specified, setting to %s", outFile)
	}
	if !path.IsAbs(outFile) {
		outFile = path.Join(os.Getenv("HOME"), ".local/bin/", outFile)
		log.Debugf("outFile set to path %s", outFile)
	}
	log.WithField("outFile", outFile).Debug("outfile")

	gh := NewGithubRepo(owner, repo)
	err := gh.GetRelease(ctx, githubPath)
	if err != nil {
		return err
	}

	fileGlob = expandVariables(gh.release, fileGlob)
	globs := strings.Split(fileGlob, "!")

	asset, err := getAssetByGlob(gh.release, globs[0])
	if err != nil {
		return err
	}
	artifact, err := gh.newArtifactFromAsset(gh.release, asset, checksumType)
	if err != nil {
		return err
	}
	artifact.FromGlob = globs[0]
	gh.Manifest.Artifacts = append(gh.Manifest.Artifacts, artifact)

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

		fmt.Printf("Installing from %s\n", artifact.RemoteFile)

		err = InstallFile(artifact, f, outFile, strings.Join(globs[1:], "!"))
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
		if artifact.ChecksumType == "" {
			artifact.ChecksumType = ChecksumShasum256
		}
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
					globs := expandVariables(repo.release, ia.FromGlob)
					err = InstallFile(newArtifact, f, ia.LocalFile, globs)
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
		return nil
	}
	fmt.Printf("  upgrade %s -> %s\n", currentVersion, repo.Manifest.CurrentVersion)
	return nil
}

func getChecksumFile(release *github.RepositoryRelease, asset *github.ReleaseAsset, checksumType string) (*github.ReleaseAsset, error) {
	checksumTypes := strings.Split(checksumType, "!")
	checksumGlob := ""
	if len(checksumTypes) > 1 {
		checksumGlob = checksumTypes[1]
	}
	if checksumTypes[0] == ChecksumShasum256 {
		if checksumGlob == "" {
			checksumGlob = "sha256sum"
		}
	} else if strings.HasPrefix(checksumTypes[0], "per_asset:") {
		if checksumGlob == "" {
			checksumGlob = asset.GetName() + "." + strings.TrimPrefix(checksumType, "per_asset:")
		}
	} else {
		return nil, errors.Errorf("unsupported checksum type %s for github module", checksumType)
	}
	checksumGlob = expandVariables(release, checksumGlob)
	log.WithFields(log.Fields{
		"asset": asset.GetName(),
		"type":  checksumTypes[0],
		"glob":  checksumGlob,
	}).Debug("looking for matching checksum file")
	asset, err := getAssetByGlob(release, checksumGlob)
	return asset, err
}

func NewGithubRepo(owner string, repo string) *GithubRepo {

	m := NewBinmgrManifest()
	m.Type = "github"
	m.Properties = map[string]string{
		"owner": owner,
		"repo":  repo,
	}
	r := GithubRepo{
		Manifest: m,
		log:      log.WithField("owner", owner).WithField("repo", repo),
		client:   newGithubClient(),
		owner:    owner,
		repo:     repo,
	}
	return &r
}

func NewGithubRepoFromManifest(manifest *BinmgrManifest) *GithubRepo {
	owner := manifest.Properties["owner"]
	repo := manifest.Properties["repo"]
	r := GithubRepo{
		Manifest: manifest,
		log:      log.WithField("owner", owner).WithField("repo", repo),
		client:   newGithubClient(),
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
	return nil
}

func (g *GithubRepo) newArtifactFromAsset(release *github.RepositoryRelease, asset *github.ReleaseAsset, checksumType string) (*Artifact, error) {
	checksums := make(map[string]string, 0)
	checksumTypes := strings.Split(checksumType, "!")
	checksumUrl := ""
	switch checksumTypes[0] {
	case ChecksumShasum256:
		checksumGlob := "sha256sum"
		if len(checksumTypes) > 1 && checksumTypes[1] != "" {
			checksumGlob = checksumTypes[1]
			checksumGlob = expandVariables(release, checksumGlob)
		}
		checksumFile, err := getAssetByGlob(release, checksumGlob)
		if err != nil {
			log.WithError(err).Error("failed to retrieve checksum file")
			return nil, err
		}
		checksumUrl = checksumFile.GetBrowserDownloadURL()
		log.WithField("file", checksumFile).WithField("url", checksumUrl).Debug("got checksum file")
		checksum, err := GetSumForFile(nil, checksumUrl, asset.GetName())
		if err != nil {
			return nil, err
		}
		checksums[ChecksumShasum256] = checksum
	case ChecksumMultiSum:
		checksumGlob := "checksums"
		if len(checksumTypes) > 1 && checksumTypes[1] != "" {
			checksumGlob = checksumTypes[1]
		}
		mapGlob := "checksums_hashes_order"
		if len(checksumTypes) > 2 && checksumTypes[2] != "" {
			mapGlob = checksumTypes[2]
		}
		checksumFile, err := getAssetByGlob(release, checksumGlob)
		if err != nil {
			return nil, err
		}
		mapFile, err := getAssetByGlob(release, mapGlob)
		if err != nil {
			return nil, err
		}
		checksumUrl = checksumFile.GetBrowserDownloadURL()
		csumMap, err := GetChecksumMap(nil, mapFile.GetBrowserDownloadURL())
		if err != nil {
			return nil, err
		}
		log.WithField("map", csumMap).Debug("retrieved checksum map")
		checksums, err = GetMultiSumForFile(nil, csumMap, checksumUrl, asset.GetName())
		if err != nil {
			return nil, err
		}
	}
	log.WithField("checksums", checksums).Debug("retrieved checksums")

	artifact := NewArtifact()
	artifact.AssetUrl = *asset.URL
	artifact.RemoteFile = asset.GetBrowserDownloadURL()
	artifact.ChecksumType = checksumType
	artifact.ChecksumFile = checksumUrl
	artifact.Checksums = checksums
	artifact.Installed = false
	return artifact, nil
}

func (g *GithubRepo) UpdateArtifact(a *Artifact) (*Artifact, error) {
	asset, err := getAssetByGlob(g.release, a.FromGlob)
	if err != nil {
		log.WithError(err).Error("failed to find asset by glob")
		return nil, err
	}
	artifact, err := g.newArtifactFromAsset(g.release, asset, a.ChecksumType)
	if err != nil {
		log.WithError(err).Error("failed to create new artifact from asset")
		return nil, err
	}
	artifact.FromGlob = a.FromGlob
	return artifact, err
}

func getAssetByGlob(release *github.RepositoryRelease, nameGlob string) (*github.ReleaseAsset, error) {
	for _, asset := range release.Assets {
		t, err := filepath.Match(nameGlob, asset.GetName())
		log.WithFields(log.Fields{
			"glob":  nameGlob,
			"match": t,
			"asset": asset.GetName(),
		}).Debug("asset")
		if err != nil {
			return nil, err
		}
		if t {
			return asset, nil
		}
	}
	return nil, errors.Errorf("no asset found matching %s", nameGlob)
}
