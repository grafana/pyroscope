package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/google/go-github/v58/github"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/oauth2"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
)

// GithubClient returns a github client.
func GithubClient(ctx context.Context, token *oauth2.Token, apiDuration *prometheus.HistogramVec, apiRateLimit prometheus.Gauge) (*githubClient, error) {
	return &githubClient{
		client:       github.NewClient(nil).WithAuthToken(token.AccessToken),
		apiDuration:  apiDuration,
		apiRateLimit: apiRateLimit,
	}, nil
}

type githubClient struct {
	client *github.Client

	apiDuration  *prometheus.HistogramVec
	apiRateLimit prometheus.Gauge
}

func (gh *githubClient) GetCommit(ctx context.Context, owner, repo, ref string) (*vcsv1.GetCommitResponse, error) {
	start := time.Now()
	commit, res, err := gh.client.Repositories.GetCommit(ctx, owner, repo, ref, nil)
	gh.apiDuration.WithLabelValues("/repos/{owner}/{repo}/commits/{ref}").Observe(time.Since(start).Seconds())
	gh.apiRateLimit.Set(float64(res.Rate.Remaining))
	if err != nil {
		return nil, err
	}

	return &vcsv1.GetCommitResponse{
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

	start := time.Now()
	file, _, res, err := gh.client.Repositories.GetContents(ctx, req.Owner, req.Repo, req.Path, &github.RepositoryContentGetOptions{Ref: req.Ref})
	gh.apiDuration.WithLabelValues("/repos/{owner}/{repo}/contents/{path}").Observe(time.Since(start).Seconds())
	gh.apiRateLimit.Set(float64(res.Rate.Remaining))
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
