package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"connectrpc.com/connect"
	"github.com/google/go-github/v58/github"
	"golang.org/x/oauth2"
	o2endpoints "golang.org/x/oauth2/endpoints"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
)

var (
	GithubAppClientID     = os.Getenv("GITHUB_CLIENT_ID")
	githubAppClientSecret = os.Getenv("GITHUB_CLIENT_SECRET")
	githubSessionSecret   = []byte(os.Getenv("GITHUB_SESSION_SECRET"))
)

const (
	gitHubCookieName = "GitSession"
)

// githubOAuth returns a github oauth2 config.
// Returns an error if the environment variables are not set.
func githubOAuth() (*oauth2.Config, error) {
	if GithubAppClientID == "" {
		return nil, errors.New("missing GITHUB_CLIENT_ID environment variable")
	}
	if githubAppClientSecret == "" {
		return nil, errors.New("missing GITHUB_CLIENT_SECRET environment variable")
	}
	return &oauth2.Config{
		ClientID:     GithubAppClientID,
		ClientSecret: githubAppClientSecret,
		Endpoint:     o2endpoints.GitHub,
	}, nil
}

// GithubClient returns a github client for the given request headers.
func GithubClient(ctx context.Context, requestHeaders http.Header) (*githubClient, error) {
	auth, err := githubOAuth()
	if err != nil {
		return nil, err
	}
	cookie, err := (&http.Request{Header: requestHeaders}).Cookie(gitHubCookieName)
	if err != nil {
		return nil, err
	}
	token, err := decryptToken(cookie.Value, githubSessionSecret)
	if err != nil {
		return nil, unAuthorizeError(err, cookie)
	}
	if !token.Valid() {
		return nil, unAuthorizeError(errors.New("invalid or expired token"), cookie)
	}
	return &githubClient{
		client: github.NewClient(auth.Client(ctx, token)),
	}, nil
}

func unAuthorizeError(err error, cookie *http.Cookie) error {
	connectErr := connect.NewError(
		connect.CodeUnauthenticated,
		err,
	)
	cookie.Value = ""
	cookie.MaxAge = -1
	connectErr.Meta().Set("Set-Cookie", cookie.String())
	return connectErr
}

func AuthorizeGithub(ctx context.Context, authorizationCode string) (string, error) {
	auth, err := githubOAuth()
	if err != nil {
		return "", err
	}
	token, err := auth.Exchange(ctx, authorizationCode)
	if err != nil {
		return "", err
	}
	cookieValue, err := encryptToken(token, githubSessionSecret)
	if err != nil {
		return "", err
	}
	// Sets a cookie with the encrypted token.
	// Only the server can decrypt the cookie.
	cookie := http.Cookie{
		Name:     gitHubCookieName,
		Value:    cookieValue,
		Expires:  token.Expiry.Add(-10 * time.Second),
		HttpOnly: false,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
	return cookie.String(), nil
}

type githubClient struct {
	client *github.Client
}

func (gh *githubClient) GetCommit(ctx context.Context, owner, repo, ref string) (*vcsv1.GetCommitResponse, error) {
	commit, _, err := gh.client.Repositories.GetCommit(ctx, owner, repo, ref, nil)
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
	file, _, _, err := gh.client.Repositories.GetContents(ctx, req.Owner, req.Repo, req.Path, &github.RepositoryContentGetOptions{Ref: req.Ref})
	if err != nil {
		var githubErr *github.ErrorResponse
		if ok := errors.As(err, &githubErr); ok && githubErr.Response.StatusCode == http.StatusNotFound {
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
