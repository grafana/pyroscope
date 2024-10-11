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
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/oauth2"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1/vcsv1connect"
	"github.com/grafana/pyroscope/pkg/querier/vcs/client"
	"github.com/grafana/pyroscope/pkg/querier/vcs/source"
)

var _ vcsv1connect.VCSServiceHandler = (*Service)(nil)

type Service struct {
	logger     log.Logger
	httpClient *http.Client
}

func New(logger log.Logger, reg prometheus.Registerer) *Service {
	httpClient := client.InstrumentedHTTPClient(logger, reg)

	return &Service{
		logger:     logger,
		httpClient: httpClient,
	}
}

func (q *Service) GithubApp(ctx context.Context, req *connect.Request[vcsv1.GithubAppRequest]) (*connect.Response[vcsv1.GithubAppResponse], error) {
	err := isGitHubIntegrationConfigured()
	if err != nil {
		q.logger.Log("err", err, "msg", "GitHub integration is not configured")
		return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("GitHub integration is not configured"))
	}

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

	cookie, err := encodeTokenInCookie(token, encryptionKey)
	if err != nil {
		q.logger.Log("err", err, "msg", "failed to encode deprecated GitHub OAuth token")
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to authorize with GitHub"))
	}

	encoded, err := encryptToken(token, encryptionKey)
	if err != nil {
		q.logger.Log("err", err, "msg", "failed to encode GitHub OAuth token")
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to authorize with GitHub"))
	}

	res := &vcsv1.GithubLoginResponse{
		Cookie:                cookie.String(),
		Token:                 encoded,
		TokenExpiresAt:        token.Expiry.UnixMilli(),
		RefreshTokenExpiresAt: time.Now().Add(githubRefreshExpiryDuration).UnixMilli(),
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

	githubToken, err := refreshGithubToken(githubRequest, q.httpClient)
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

	cookie, err := encodeTokenInCookie(newToken, derivedKey)
	if err != nil {
		q.logger.Log("err", err, "msg", "failed to encode deprecated GitHub OAuth token")
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to refresh token"))
	}

	encoded, err := encryptToken(newToken, derivedKey)
	if err != nil {
		q.logger.Log("err", err, "msg", "failed to encode GitHub OAuth token")
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to refresh token"))
	}

	res := &vcsv1.GithubRefreshResponse{
		Cookie:                cookie.String(),
		Token:                 encoded,
		TokenExpiresAt:        token.Expiry.UnixMilli(),
		RefreshTokenExpiresAt: time.Now().Add(githubRefreshExpiryDuration).UnixMilli(),
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
	ghClient, err := client.GithubClient(ctx, token, q.httpClient)
	if err != nil {
		return nil, err
	}

	file, err := source.NewFileFinder(
		ghClient,
		gitURL,
		req.Msg.LocalPath,
		req.Msg.RootPath,
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

	ghClient, err := client.GithubClient(ctx, token, q.httpClient)
	if err != nil {
		return nil, err
	}

	owner := gitURL.GetOwnerName()
	repo := gitURL.GetRepoName()
	ref := req.Msg.GetRef()

	commit, err := tryGetCommit(ctx, ghClient, owner, repo, ref)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&vcsv1.GetCommitResponse{
		Message: commit.GetMessage(),
		Author:  commit.GetAuthor(),
		Date:    commit.GetDate(),
		Sha:     commit.GetSha(),
		URL:     commit.GetURL(),
	}), nil
}

func (q *Service) GetCommits(ctx context.Context, req *connect.Request[vcsv1.GetCommitsRequest]) (*connect.Response[vcsv1.GetCommitsResponse], error) {
	token, err := tokenFromRequest(ctx, req)
	if err != nil {
		q.logger.Log("err", err, "msg", "failed to extract token from request")
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid token"))
	}

	err = rejectExpiredToken(token)
	if err != nil {
		return nil, err
	}

	gitURL, err := giturl.NewGitURL(req.Msg.RepositoryUrl)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if gitURL.GetProvider() != apis.ProviderGitHub.String() {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("only GitHub repositories are supported"))
	}

	ghClient, err := client.GithubClient(ctx, token, q.httpClient)
	if err != nil {
		return nil, err
	}

	owner := gitURL.GetOwnerName()
	repo := gitURL.GetRepoName()
	refs := req.Msg.Refs

	commits, failedFetches, err := getCommits(ctx, ghClient, owner, repo, refs)
	if err != nil {
		q.logger.Log("err", err, "msg", "failed to get any commits", "owner", owner, "repo", repo)
		return nil, err
	}

	if len(failedFetches) > 0 {
		q.logger.Log("warn", "partial success fetching commits", "owner", owner, "repo", repo, "successCount", len(commits), "failureCount", len(failedFetches))
		for _, fetchErr := range failedFetches {
			q.logger.Log("err", fetchErr, "msg", "failed to fetch commit")
		}
	}

	return connect.NewResponse(&vcsv1.GetCommitsResponse{Commits: commits}), nil
}

func rejectExpiredToken(token *oauth2.Token) error {
	if time.Now().After(token.Expiry) {
		return connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("token is expired"))
	}
	return nil
}
