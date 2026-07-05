package usecase

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	domainplugin "ssh-client/internal/domain/plugin"
)

func (s *GitHubPluginService) downloadBinary(
	ctx context.Context,
	owner, repo, tag, assetName, checksum string,
) (path string, cleanup func(), err error) {
	path, err = s.downloader.DownloadBinary(ctx, owner, repo, tag, assetName, checksum)
	if err != nil {
		return "", func() {}, err
	}
	root := findTempRoot(path)
	return path, func() { _ = os.RemoveAll(root) }, nil
}

func findTempRoot(path string) string {
	dir := filepath.Dir(path)
	for i := 0; i < 5; i++ {
		if strings.Contains(dir, "xqsp-") || strings.Contains(dir, "xqs-github-stage-") {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return filepath.Dir(path)
}

func (s *GitHubPluginService) loadReleaseChecksums(ctx context.Context, owner, repo string, release *domainplugin.GitHubRelease) map[string]string {
	if release == nil {
		return nil
	}
	for _, asset := range release.Assets {
		if asset.Name != "SHA256SUMS" && asset.Name != "checksums.txt" {
			continue
		}
		path, cleanup, err := s.downloadBinary(ctx, owner, repo, release.TagName, asset.Name, "")
		if err != nil {
			continue
		}
		data, readErr := os.ReadFile(path)
		cleanup()
		if readErr != nil {
			continue
		}
		return domainplugin.ParseChecksumsFile(string(data))
	}
	return nil
}
