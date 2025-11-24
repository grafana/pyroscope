package source

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	giturl "github.com/kubescape/go-git-url"
	"github.com/opentracing/opentracing-go"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
	"github.com/grafana/pyroscope/pkg/frontend/vcs/client"
	"github.com/grafana/pyroscope/pkg/frontend/vcs/config"
)

type VCSClient interface {
	GetFile(ctx context.Context, req client.FileRequest) (client.File, error)
}

// FileFinder finds a file in a vcs repository.
type FileFinder struct {
	file          config.FileSpec
	ref, rootPath string
	repo          giturl.IGitURL

	config     *config.PyroscopeConfig
	client     VCSClient
	httpClient *http.Client
	logger     log.Logger
}

// NewFileFinder returns a new FileFinder.
func NewFileFinder(client VCSClient, repo giturl.IGitURL, file config.FileSpec, rootPath, ref string, httpClient *http.Client, logger log.Logger) *FileFinder {
	if ref == "" {
		ref = "HEAD"
	}
	return &FileFinder{
		client:     client,
		logger:     logger,
		repo:       repo,
		file:       file,
		rootPath:   rootPath,
		ref:        ref,
		httpClient: httpClient,
	}
}

// Find returns the file content and URL.
func (ff *FileFinder) Find(ctx context.Context) (*vcsv1.GetFileResponse, error) {
	// first try to gather the config
	ff.loadConfig(ctx)

	// without config we are done here
	if ff.config == nil {
		return ff.findFallback(ctx)
	}

	// find matching mappings
	mapping := ff.config.FindMapping(ff.file)
	if mapping == nil {
		return ff.findFallback(ctx)
	}

	switch config.Language(mapping.Language) {
	case config.LanguageGo:
		return ff.findGoFile(ctx, mapping)
	case config.LanguageJava:
		return ff.findJavaFile(ctx, mapping)
	// todo: add more languages support
	default:
		return ff.findFallback(ctx)
	}
}

func (ff FileFinder) findFallback(ctx context.Context) (*vcsv1.GetFileResponse, error) {
	switch filepath.Ext(ff.file.Path) {
	case ExtGo:
		return ff.findGoFile(ctx)
	case ExtAsm: // Note: When adding wider language support this needs to be revisited
		return ff.findGoFile(ctx)
	// todo: add more languages support
	default:
		// by default we return the file content at the given path without any processing.
		content, err := ff.fetchRepoFile(ctx, ff.file.Path, ff.ref)
		if err != nil {
			return nil, err
		}
		return newFileResponse(content.Content, content.URL)
	}
}

// loadConfig attempts to load .pyroscope.yaml from the repository root
func (ff *FileFinder) loadConfig(ctx context.Context) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "FileFinder.loadConfig")
	defer sp.Finish()

	configPath := config.PyroscopeConfigPath
	if ff.rootPath != "" {
		configPath = filepath.Join(ff.rootPath, config.PyroscopeConfigPath)
	}

	file, err := ff.client.GetFile(ctx, client.FileRequest{
		Owner: ff.repo.GetOwnerName(),
		Repo:  ff.repo.GetRepoName(),
		Path:  configPath,
		Ref:   ff.ref,
	})
	if err != nil {
		// Config is optional, so just log and continue
		level.Debug(ff.logger).Log("msg", "no .pyroscope.yaml found", "path", configPath)
		return
	}
	sp.SetTag("config.url", file.URL)
	sp.SetTag("config", file.Content)

	cfg, err := config.ParsePyroscopeConfig([]byte(file.Content))
	if err != nil {
		level.Warn(ff.logger).Log("msg", "failed to parse .pyroscope.yaml", "err", err)
		return
	}

	ff.config = cfg
	level.Debug(ff.logger).Log("msg", "loaded .pyroscope.yaml", "url", file.URL, "mappings", len(cfg.SourceCode.Mappings))
	sp.SetTag("config.source_code.mappings_count", len(cfg.SourceCode.Mappings))

}

// fetchRepoFile fetches the file content from the configured repository.
func (arg FileFinder) fetchRepoFile(ctx context.Context, path, ref string) (*vcsv1.GetFileResponse, error) {
	if arg.rootPath != "" {
		path = filepath.Join(arg.rootPath, path)
	}
	content, err := arg.client.GetFile(ctx, client.FileRequest{
		Owner: arg.repo.GetOwnerName(),
		Repo:  arg.repo.GetRepoName(),
		Path:  strings.TrimLeft(path, "/"),
		Ref:   ref,
	})
	if err != nil {
		return nil, err
	}
	return newFileResponse(content.Content, content.URL)
}

// fetchURL fetches the file content from the given URL.
func (ff FileFinder) fetchURL(ctx context.Context, url string, decodeBase64 bool) (*vcsv1.GetFileResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := ff.httpClient.Do(req) // todo: use a custom client with timeout
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("failed to fetch %s: %s", url, resp.Status))
	}
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if !decodeBase64 {
		return newFileResponse(string(content), url)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(content))
	if err != nil {
		return nil, err
	}
	return newFileResponse(string(decoded), url)
}

func newFileResponse(content, url string) (*vcsv1.GetFileResponse, error) {
	return &vcsv1.GetFileResponse{
		Content: base64.StdEncoding.EncodeToString([]byte(content)),
		URL:     url,
	}, nil
}

func (ff FileFinder) fetchMappingFile(ctx context.Context, m *config.MappingConfig, path string) (*vcsv1.GetFileResponse, error) {
	if s := m.Source.Local; s != nil {
		if s.Path != "" {
			path = filepath.Join(s.Path, path)
		}
		return ff.fetchRepoFile(ctx, path, ff.ref)
	}
	if s := m.Source.GitHub; s != nil {
		if s.Path != "" {
			path = filepath.Join(s.Path, path)
		}
		content, err := ff.client.GetFile(ctx, client.FileRequest{
			Owner: s.Owner,
			Repo:  s.Repo,
			Ref:   s.Ref,
			Path:  path,
		})
		if err != nil {
			return nil, err
		}
		return newFileResponse(content.Content, content.URL)
	}
	return nil, fmt.Errorf("no supported source provided, file not resolvable")
}
