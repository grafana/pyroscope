package vcs

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	giturl "github.com/kubescape/go-git-url"
	"github.com/kubescape/go-git-url/apis"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
	vcsv1connect "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1/vcsv1connect"
	client "github.com/grafana/pyroscope/pkg/querier/vcs/client"
	"github.com/grafana/pyroscope/pkg/querier/vcs/source"
)

var _ vcsv1connect.VCSServiceHandler = (*Service)(nil)

type Service struct {
	logger log.Logger
}

func New(logger log.Logger) *Service {
	return &Service{
		logger: logger,
	}
}

func (q *Service) GithubApp(ctx context.Context, req *connect.Request[vcsv1.GithubAppRequest]) (*connect.Response[vcsv1.GithubAppResponse], error) {
	return connect.NewResponse(&vcsv1.GithubAppResponse{
		ClientID: client.GithubAppClientID,
	}), nil
}

func (q *Service) GithubLogin(ctx context.Context, req *connect.Request[vcsv1.GithubLoginRequest]) (*connect.Response[vcsv1.GithubLoginResponse], error) {
	resp := connect.NewResponse(&vcsv1.GithubLoginResponse{})
	cookie, err := client.AuthorizeGithub(ctx, req.Msg.AuthorizationCode)
	if err != nil {
		return nil, fmt.Errorf("failed to authorize github: %w", err)
	}
	resp.Msg.Cookie = cookie
	return resp, nil
}

func (q *Service) GetFile(ctx context.Context, req *connect.Request[vcsv1.GetFileRequest]) (*connect.Response[vcsv1.GetFileResponse], error) {
	// initialize and parse the git repo URL
	gitURL, err := giturl.NewGitURL(req.Msg.RepositoryURL)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if gitURL.GetProvider() != apis.ProviderGitHub.String() {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("only GitHub repositories are supported"))
	}

	// todo: we can support multiple provider: bitbucket, gitlab, etc.
	ghClient, err := client.GithubClient(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	file, err := source.
		NewFileFinder(
			ghClient,
			gitURL,
			req.Msg.LocalPath,
			req.Msg.Ref,
			http.DefaultClient,
			log.With(q.logger, "repo", gitURL.GetRepoName())).
		Find(ctx)
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, err
	}
	return connect.NewResponse(file), nil
}

func (q *Service) GetCommit(ctx context.Context, req *connect.Request[vcsv1.GetCommitRequest]) (*connect.Response[vcsv1.GetCommitResponse], error) {
	gitURL, err := giturl.NewGitURL(req.Msg.RepositoryURL)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if gitURL.GetProvider() != apis.ProviderGitHub.String() {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("only GitHub repositories are supported"))
	}
	ghClient, err := client.GithubClient(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	commit, err := ghClient.GetCommit(ctx, gitURL.GetOwnerName(), gitURL.GetRepoName(), req.Msg.Ref)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(commit), nil
}
