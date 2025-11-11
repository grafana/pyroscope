package source

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	giturl "github.com/kubescape/go-git-url"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
	"github.com/grafana/pyroscope/pkg/frontend/vcs/client"
	"github.com/grafana/pyroscope/pkg/frontend/vcs/config"
)

const pyroscopeConfigFile = ".pyroscope.yaml"

// ConfigAwareFileFinder wraps FileFinder and adds support for .pyroscope.yaml configuration
type ConfigAwareFileFinder struct {
	*FileFinder
	config *config.PyroscopeConfig
}

// NewConfigAwareFileFinder creates a new ConfigAwareFileFinder
// It attempts to load .pyroscope.yaml from the repository root
func NewConfigAwareFileFinder(client VCSClient, repo giturl.IGitURL, path, rootPath, ref string, httpClient *http.Client, logger log.Logger) *ConfigAwareFileFinder {
	ff := NewFileFinder(client, repo, path, rootPath, ref, httpClient, logger)

	caf := &ConfigAwareFileFinder{
		FileFinder: ff,
	}

	return caf
}

// Find returns the file content and URL, using .pyroscope.yaml mappings if available
func (caf *ConfigAwareFileFinder) Find(ctx context.Context) (*vcsv1.GetFileResponse, error) {
	// Try to load config if not already loaded
	if caf.config == nil {
		caf.loadConfig(ctx)
	}

	// If we have a config, try to use it for path resolution
	if caf.config != nil {
		if resolved, err := caf.findWithConfig(ctx); err == nil {
			return resolved, nil
		} else if !errors.Is(err, client.ErrNotFound) {
			// Log non-NotFound errors but continue with fallback
			level.Warn(caf.logger).Log("msg", "failed to resolve file with config", "err", err)
		}
	}

	// Fall back to standard file finding
	return caf.FileFinder.Find(ctx)
}

// loadConfig attempts to load .pyroscope.yaml from the repository root
func (caf *ConfigAwareFileFinder) loadConfig(ctx context.Context) {
	configPath := pyroscopeConfigFile
	if caf.rootPath != "" {
		configPath = filepath.Join(caf.rootPath, pyroscopeConfigFile)
	}

	file, err := caf.client.GetFile(ctx, client.FileRequest{
		Owner: caf.repo.GetOwnerName(),
		Repo:  caf.repo.GetRepoName(),
		Path:  configPath,
		Ref:   caf.ref,
	})
	if err != nil {
		// Config is optional, so just log and continue
		level.Debug(caf.logger).Log("msg", "no .pyroscope.yaml found", "path", configPath)
		return
	}

	cfg, err := config.ParsePyroscopeConfig([]byte(file.Content))
	if err != nil {
		level.Warn(caf.logger).Log("msg", "failed to parse .pyroscope.yaml", "err", err)
		return
	}

	caf.config = cfg
	level.Info(caf.logger).Log("msg", "loaded .pyroscope.yaml", "language", cfg.SourceCode.Language, "mappings", len(cfg.SourceCode.Mappings))
}

// findWithConfig attempts to resolve the file using config mappings
func (caf *ConfigAwareFileFinder) findWithConfig(ctx context.Context) (*vcsv1.GetFileResponse, error) {
	// Clean the path for matching
	cleanPath := caf.path
	if caf.rootPath != "" {
		cleanPath = strings.TrimPrefix(cleanPath, caf.rootPath)
		cleanPath = strings.TrimPrefix(cleanPath, "/")
	}

	// Find the best matching mapping
	mapping := caf.config.FindMapping(cleanPath)
	if mapping == nil {
		level.Debug(caf.logger).Log("msg", "no mapping found for path", "path", cleanPath)
		return nil, client.ErrNotFound
	}

	level.Debug(caf.logger).Log("msg", "found mapping", "path", cleanPath, "mapping_path", mapping.Path, "type", mapping.Type)

	switch mapping.Type {
	case "local":
		return caf.resolveLocalMapping(ctx, cleanPath, mapping)
	case "github":
		return caf.resolveGitHubMapping(ctx, cleanPath, mapping)
	default:
		return nil, connect.NewError(connect.CodeInternal, errors.New("unsupported mapping type"))
	}
}

// resolveLocalMapping resolves a file using a local mapping
func (caf *ConfigAwareFileFinder) resolveLocalMapping(ctx context.Context, cleanPath string, mapping *config.MappingConfig) (*vcsv1.GetFileResponse, error) {
	// Replace the mapping prefix with the local path
	relativePath := strings.TrimPrefix(cleanPath, mapping.Path)
	relativePath = strings.TrimPrefix(relativePath, "/")

	targetPath := mapping.Local.Path
	if relativePath != "" {
		targetPath = filepath.Join(mapping.Local.Path, relativePath)
	}

	// Add rootPath if present
	if caf.rootPath != "" {
		targetPath = filepath.Join(caf.rootPath, targetPath)
	}

	level.Debug(caf.logger).Log("msg", "resolving local mapping", "original", cleanPath, "target", targetPath)

	file, err := caf.client.GetFile(ctx, client.FileRequest{
		Owner: caf.repo.GetOwnerName(),
		Repo:  caf.repo.GetRepoName(),
		Path:  targetPath,
		Ref:   caf.ref,
	})
	if err != nil {
		return nil, err
	}

	return newFileResponse(file.Content, file.URL)
}

// resolveGitHubMapping resolves a file using a GitHub mapping
func (caf *ConfigAwareFileFinder) resolveGitHubMapping(ctx context.Context, cleanPath string, mapping *config.MappingConfig) (*vcsv1.GetFileResponse, error) {
	// Replace the mapping prefix with the GitHub path
	relativePath := strings.TrimPrefix(cleanPath, mapping.Path)
	relativePath = strings.TrimPrefix(relativePath, "/")

	targetPath := mapping.GitHub.Path
	if relativePath != "" {
		targetPath = filepath.Join(mapping.GitHub.Path, relativePath)
	}

	level.Debug(caf.logger).Log(
		"msg", "resolving github mapping",
		"original", cleanPath,
		"target_repo", mapping.GitHub.Owner+"/"+mapping.GitHub.Repo,
		"target_path", targetPath,
		"ref", mapping.GitHub.Ref,
	)

	// Fetch from the mapped GitHub repository
	file, err := caf.client.GetFile(ctx, client.FileRequest{
		Owner: mapping.GitHub.Owner,
		Repo:  mapping.GitHub.Repo,
		Path:  targetPath,
		Ref:   mapping.GitHub.Ref,
	})
	if err != nil {
		return nil, err
	}

	// Build the URL for the mapped repository
	url := file.URL

	// Encode the content as base64
	encoded := base64.StdEncoding.EncodeToString([]byte(file.Content))

	return &vcsv1.GetFileResponse{
		Content: encoded,
		URL:     url,
	}, nil
}
