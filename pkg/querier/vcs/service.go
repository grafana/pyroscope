package vcs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	giturl "github.com/kubescape/go-git-url"
	"github.com/kubescape/go-git-url/apis"
	"golang.org/x/oauth2"

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
		ClientID: githubAppClientID,
	}), nil
}

func (q *Service) GithubLogin(ctx context.Context, req *connect.Request[vcsv1.GithubLoginRequest]) (*connect.Response[vcsv1.GithubLoginResponse], error) {
	cfg, err := githubOAuthConfig()
	if err != nil {
		q.logger.Log("err", err, "msg", "failed to get GitHub OAuth config")
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to authorize with GitHub"))
	}

	encryptionKey, err := deriveEncryptionKeyForContext(ctx)
	if err != nil {
		q.logger.Log("err", err, "msg", "failed to derive encryption key")
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to authorize with GitHub"))
	}

	token, err := cfg.Exchange(ctx, req.Msg.AuthorizationCode)
	if err != nil {
		q.logger.Log("err", err, "msg", "failed to exchange authorization code with GitHub")
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("failed to authorize with GitHub"))
	}

	cookie, err := encodeToken(token, encryptionKey)
	if err != nil {
		q.logger.Log("err", err, "msg", "failed to encode GitHub OAuth token")
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to authorize with GitHub"))
	}

	res := &vcsv1.GithubLoginResponse{
		Cookie: cookie.String(),
	}
	return connect.NewResponse(res), nil
}

func (q *Service) GithubRefresh(ctx context.Context, req *connect.Request[vcsv1.GithubRefreshRequest]) (*connect.Response[vcsv1.GithubRefreshResponse], error) {
	token, err := tokenFromRequest(ctx, req)
	if err != nil {
		q.logger.Log("err", err, "msg", "failed to extract token from request")
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid token"))
	}

	githubRequest, err := buildGithubRefreshRequest(ctx, token)
	if err != nil {
		q.logger.Log("err", err, "msg", "failed to extract token from request")
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to refresh token"))
	}

	githubToken, err := refreshGithubToken(githubRequest)
	if err != nil {
		q.logger.Log("err", err, "msg", "failed to refresh token with GitHub")
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to refresh token"))
	}

	newToken := githubToken.toOAuthToken()

	derivedKey, err := deriveEncryptionKeyForContext(ctx)
	if err != nil {
		q.logger.Log("err", err, "msg", "failed to derive encryption key")
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to process token"))
	}

	cookie, err := encodeToken(newToken, derivedKey)
	if err != nil {
		q.logger.Log("err", err, "msg", "failed to encode GitHub OAuth token")
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to refresh token"))
	}

	res := &vcsv1.GithubRefreshResponse{
		Cookie: cookie.String(),
	}
	return connect.NewResponse(res), nil
}

func (q *Service) GetFile(ctx context.Context, req *connect.Request[vcsv1.GetFileRequest]) (*connect.Response[vcsv1.GetFileResponse], error) {
	token, err := tokenFromRequest(ctx, req)
	if err != nil {
		q.logger.Log("err", err, "msg", "failed to extract token from request")
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid token"))
	}

	err = rejectExpiredToken(token)
	if err != nil {
		return nil, err
	}

	// initialize and parse the git repo URL
	gitURL, err := giturl.NewGitURL(req.Msg.RepositoryURL)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if gitURL.GetProvider() != apis.ProviderGitHub.String() {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("only GitHub repositories are supported"))
	}

	// todo: we can support multiple provider: bitbucket, gitlab, etc.
	ghClient, err := client.GithubClient(ctx, token)
	if err != nil {
		return nil, err
	}

	file, err := source.NewFileFinder(
		ghClient,
		gitURL,
		req.Msg.LocalPath,
		req.Msg.Ref,
		http.DefaultClient,
		log.With(q.logger, "repo", gitURL.GetRepoName()),
	).Find(ctx)
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, err
	}
	return connect.NewResponse(file), nil
}

func (q *Service) GetCommit(ctx context.Context, req *connect.Request[vcsv1.GetCommitRequest]) (*connect.Response[vcsv1.GetCommitResponse], error) {
	token, err := tokenFromRequest(ctx, req)
	if err != nil {
		q.logger.Log("err", err, "msg", "failed to extract token from request")
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid token"))
	}

	err = rejectExpiredToken(token)
	if err != nil {
		return nil, err
	}

	gitURL, err := giturl.NewGitURL(req.Msg.RepositoryURL)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if gitURL.GetProvider() != apis.ProviderGitHub.String() {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("only GitHub repositories are supported"))
	}

	ghClient, err := client.GithubClient(ctx, token)
	if err != nil {
		return nil, err
	}

	commit, err := ghClient.GetCommit(ctx, gitURL.GetOwnerName(), gitURL.GetRepoName(), req.Msg.Ref)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(commit), nil
}

func rejectExpiredToken(token *oauth2.Token) error {
	if time.Now().After(token.Expiry) {
		return connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("token is expired"))
	}
	return nil
}
