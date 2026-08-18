package main

import (
	"log"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	infracache "xquakshell/internal/infra/cache"
	infraforge "xquakshell/internal/infra/forge"
	infrapersistence "xquakshell/internal/infra/persistence"
	infraplugin "xquakshell/internal/infra/plugin"
	"xquakshell/internal/usecase"
)

// Installing plugins from GitHub: the repository list, the cache, the downloader and the stager.
//
// Its own file because it is its own subject — none of it is reachable from a running plugin, and
// all of it is best-effort: a failure here leaves the feature unavailable rather than stopping the
// application from starting, which is why every step logs and continues.

// gitHubServices is the pair the runtime exposes. Either may be nil when storage failed to open.
type gitHubServices struct {
	repos   *usecase.GitHubRepositoryService
	plugins *usecase.GitHubPluginService
}

func buildGitHubServices(
	dataRoot string,
	portableData domain.PortableDataStore,
	manager *usecase.PluginManager,
) gitHubServices {
	if err := infrapersistence.EnsureGitHubReposFile(dataRoot); err != nil {
		log.Printf("WARNING: github repos file init failed: %v", err)
	}

	githubCache := infracache.NewMemoryCache(domainplugin.DefaultCacheTTL)
	githubRepoStorage, err := infrapersistence.NewFileGitHubRepositoryStorage(dataRoot)
	if err != nil {
		log.Printf("WARNING: github repo storage init failed: %v", err)
	}
	// One router serves both roles: it is the API client the plugin service calls, and the release
	// source the downloader fetches assets through. Sharing it keeps a repository registered on one
	// forge from having its metadata read there and its binaries fetched somewhere else.
	forgeRouter := infraforge.NewRouter()
	tempDir := ""
	if portableData != nil {
		if dir, err := portableData.EnsureTempDir(); err == nil {
			tempDir = dir
		} else {
			log.Printf("WARNING: portable temp dir unavailable for plugin downloads: %v", err)
		}
	}
	githubDownloader := infraplugin.NewBinaryDownloader(forgeRouter, tempDir)
	githubStager := infraplugin.NewGitHubPluginStager(tempDir)

	var githubRepoService *usecase.GitHubRepositoryService
	var githubPluginService *usecase.GitHubPluginService
	if githubRepoStorage != nil {
		githubRepoService = usecase.NewGitHubRepositoryService(githubRepoStorage, githubCache)
		githubPluginService = usecase.NewGitHubPluginService(
			forgeRouter,
			githubDownloader,
			githubStager,
			infraplugin.InstallMetaWriter{},
			githubCache,
			manager,
			githubRepoStorage,
		)
	}
	return gitHubServices{repos: githubRepoService, plugins: githubPluginService}
}
