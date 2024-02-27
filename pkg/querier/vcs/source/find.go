package source

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"path/filepath"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	giturl "github.com/kubescape/go-git-url"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
	"github.com/grafana/pyroscope/pkg/querier/vcs/client"
)

type VCSClient interface {
	GetFile(ctx context.Context, req client.FileRequest) (client.File, error)
}

// FileFinder finds a file in a vcs repository.
type FileFinder struct {
	path, ref string
	repo      giturl.IGitURL

	client     VCSClient
	httpClient *http.Client
	logger     log.Logger
}

// NewFileFinder returns a new FileFinder.
func NewFileFinder(client VCSClient, repo giturl.IGitURL, path, ref string, httpClient *http.Client, logger log.Logger) *FileFinder {
	if ref == "" {
		ref = "HEAD"
	}
	return &FileFinder{
		client:     client,
		logger:     logger,
		repo:       repo,
		path:       path,
		ref:        ref,
		httpClient: httpClient,
	}
}

// Find returns the file content and URL.
func (ff FileFinder) Find(ctx context.Context) (*vcsv1.GetFileResponse, error) {
	switch filepath.Ext(ff.path) {
	case ExtGo:
		return ff.findGoFile(ctx)
	// todo: add more languages support
	default:
		// by default we return the file content at the given path without any processing.
		content, err := ff.fetchRepoFile(ctx, ff.path, ff.ref)
		if err != nil {
			return nil, err
		}
		return newFileResponse(content.Content, content.URL)
	}
}

// fetchRepoFile fetches the file content from the configured repository.
func (arg FileFinder) fetchRepoFile(ctx context.Context, path, ref string) (*vcsv1.GetFileResponse, error) {
	content, err := arg.client.GetFile(ctx, client.FileRequest{
		Owner: arg.repo.GetOwnerName(),
		Repo:  arg.repo.GetRepoName(),
		Path:  path,
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
