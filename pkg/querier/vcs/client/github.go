package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/google/go-github/v58/github"
	"golang.org/x/oauth2"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
)

// GithubClient returns a github client.
func GithubClient(ctx context.Context, token *oauth2.Token, client *http.Client) (*githubClient, error) {
	return &githubClient{
		client: github.NewClient(client).WithAuthToken(token.AccessToken),
	}, nil
}

type githubClient struct {
	client *github.Client
}

func (gh *githubClient) GetCommit(ctx context.Context, owner, repo, ref string) (*vcsv1.CommitInfo, error) {
	commit, _, err := gh.client.Repositories.GetCommit(ctx, owner, repo, ref, nil)
	if err != nil {
		var githubErr *github.ErrorResponse
		if errors.As(err, &githubErr) {
			code := connectgrpc.HTTPToCode(int32(githubErr.Response.StatusCode))
			return nil, connect.NewError(code, err)
		}
		return nil, err
	}

	return &vcsv1.CommitInfo{
		Sha:     toString(commit.SHA),
		Message: toString(commit.Commit.Message),
		Author: &vcsv1.CommitAuthor{
			Login:     toString(commit.Author.Login),
			AvatarURL: toString(commit.Author.AvatarURL),
		},
		Date: commit.Commit.Author.Date.Format(time.RFC3339),
	}, nil
}

func (gh *githubClient) GetFile(ctx context.Context, req FileRequest) (File, error) {
	// We could abstract away git provider using git protocol
	// git clone https://x-access-token:<token>@github.com/owner/repo.git
	// For now we use the github client.

	file, _, _, err := gh.client.Repositories.GetContents(ctx, req.Owner, req.Repo, req.Path, &github.RepositoryContentGetOptions{Ref: req.Ref})
	if err != nil {
		var githubErr *github.ErrorResponse
		if errors.As(err, &githubErr) && githubErr.Response.StatusCode == http.StatusNotFound {
			return File{}, fmt.Errorf("%w: %s", ErrNotFound, err)
		}
		return File{}, err
	}

	if file == nil {
		return File{}, ErrNotFound
	}

	// We only support files retrieval.
	if file.Type != nil && *file.Type != "file" {
		return File{}, connect.NewError(connect.CodeInvalidArgument, errors.New("path is not a file"))
	}

	content, err := file.GetContent()
	if err != nil {
		return File{}, err
	}

	return File{
		Content: content,
		URL:     toString(file.HTMLURL),
	}, nil
}

func toString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
