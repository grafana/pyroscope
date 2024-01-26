package querier

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/PuerkitoBio/goquery"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/go-github/v58/github"
	giturl "github.com/kubescape/go-git-url"
	"github.com/kubescape/go-git-url/apis"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
	"golang.org/x/oauth2"
	o2endpoints "golang.org/x/oauth2/endpoints"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
	vcsv1connect "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1/vcsv1connect"
	"github.com/grafana/pyroscope/pkg/querier/golang"
	"github.com/grafana/regexp"
)

var (
	githubClientID                                    = os.Getenv("GITHUB_CLIENT_ID")
	githubClientSecret                                = os.Getenv("GITHUB_CLIENT_SECRET")
	_                  vcsv1connect.VCSServiceHandler = (*Querier)(nil)
)

// todo better package structure.
// vcs_service_test.go
// vcs_service.go
// source/find.go
// source/golang/find.go

func (q *Querier) GithubApp(ctx context.Context, req *connect.Request[vcsv1.GithubAppRequest]) (*connect.Response[vcsv1.GithubAppResponse], error) {
	return connect.NewResponse(&vcsv1.GithubAppResponse{
		ClientID: githubClientID,
	}), nil
}

func (q *Querier) GithubLogin(ctx context.Context, req *connect.Request[vcsv1.GithubLoginRequest]) (*connect.Response[vcsv1.GithubLoginResponse], error) {
	auth, err := githubOauthConfig()
	if err != nil {
		return nil, err
	}
	token, err := auth.Exchange(ctx, req.Msg.AuthorizationCode)
	if err != nil {
		return nil, err
	}
	cookieValue, err := encrypt(token)
	if err != nil {
		return nil, err
	}
	// Sets a cookie with the encrypted token.
	// Only the server can decrypt the cookie.
	cookie := http.Cookie{
		Name:  "GitSession",
		Value: cookieValue,
		// Refresh expiry is 6 months based on github docs
		Expires:  time.Now().Add(15811200 * time.Second),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
	resp := connect.NewResponse(&vcsv1.GithubLoginResponse{})
	resp.Header().Add("Set-Cookie", cookie.String())
	return resp, nil
}

func (q *Querier) GetFile(ctx context.Context, req *connect.Request[vcsv1.GetFileRequest]) (*connect.Response[vcsv1.GetFileResponse], error) {
	gitURL, err := giturl.NewGitURL(req.Msg.RepositoryURL) // initialize and parse the URL
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if gitURL.GetProvider() != apis.ProviderGitHub.String() {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("only GitHub repositories are supported"))
	}
	cookie, err := (&http.Request{Header: req.Header()}).Cookie("GitSession")
	if err != nil {
		return nil, err
	}
	// todo: we can support multiple provider: bitbucket, gitlab, etc.
	client, err := NewGithubClient(ctx, cookie)
	if err != nil {
		err := connect.NewError(
			connect.CodeUnauthenticated,
			err,
		)
		cookie.Value = ""
		cookie.MaxAge = -1
		err.Meta().Set("Set-Cookie", cookie.String())
		return nil, err
	}

	res, err := findFile(ctx, fileFinder{
		path:   req.Msg.LocalPath,
		ref:    req.Msg.Ref,
		repo:   gitURL,
		client: client,
		logger: log.With(q.logger, "repo", gitURL.GetRepoName()),
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, err
	}
	return connect.NewResponse(res), nil
}

func toString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

type githubClient struct {
	client *github.Client
}

// NewGithubClient returns a github client for the given code token exchange.
// We might want to move this to it's own package if we support multiple providers.
func NewGithubClient(ctx context.Context, cookie *http.Cookie) (*githubClient, error) {
	auth, err := githubOauthConfig()
	if err != nil {
		return nil, err
	}
	token, err := decrypt(cookie.Value)
	if err != nil {
		return nil, err
	}

	return &githubClient{
		client: github.NewClient(auth.Client(ctx, token)),
	}, nil
}

func githubOauthConfig() (*oauth2.Config, error) {
	if githubClientID == "" {
		return nil, errors.New("missing GITHUB_CLIENT_ID environment variable")
	}
	if githubClientSecret == "" {
		return nil, errors.New("missing GITHUB_CLIENT_SECRET environment variable")
	}
	return &oauth2.Config{
		ClientID:     githubClientID,
		ClientSecret: githubClientSecret,
		Endpoint:     o2endpoints.GitHub,
	}, nil
}

var ErrNotFound = errors.New("file not found")

func (gh *githubClient) GetFile(ctx context.Context, owner, repo, path, ref string) (*VCSFile, error) {
	// We could abstract away git provider using git protocol
	// git clone https://x-access-token:<token>@github.com/owner/repo.git
	// For now we use the github client.
	file, _, _, err := gh.client.Repositories.GetContents(ctx, owner, repo, path, &github.RepositoryContentGetOptions{Ref: ref})
	if err != nil {
		var githubErr *github.ErrorResponse
		if ok := errors.As(err, &githubErr); ok && githubErr.Response.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, err)
		}
		return nil, err
	}
	if file.Type != nil && *file.Type != "file" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("path is not a file"))
	}
	content, err := file.GetContent()
	if err != nil {
		return nil, err
	}
	return &VCSFile{
		Content: content,
		URL:     toString(file.DownloadURL),
	}, nil
}

type VCSFile struct {
	Content string
	URL     string
}

type VCSClient interface {
	GetFile(ctx context.Context, owner, repo, path, ref string) (*VCSFile, error)
}

// todo should be a struct with function no need to pass all of this aorund.
type fileFinder struct {
	path, ref string
	repo      giturl.IGitURL

	client VCSClient
	logger log.Logger
}

func findFile(ctx context.Context, arg fileFinder) (*vcsv1.GetFileResponse, error) {
	switch filepath.Ext(arg.path) {
	case ".go":
		return findGoFile(ctx, arg)
	// todo: we can support multiple file types: go, java, python, etc.
	default:
		// by default we return the file content at the given path without any processing.
		content, err := arg.client.GetFile(ctx, arg.repo.GetOwnerName(), arg.repo.GetRepoName(), arg.path, arg.ref)
		if err != nil {
			return nil, err
		}
		return newFileResponse(content.Content, content.URL)
	}
}

func newFileResponse(content, url string) (*vcsv1.GetFileResponse, error) {
	return &vcsv1.GetFileResponse{
		Content: base64.StdEncoding.EncodeToString([]byte(content)),
		URL:     url,
	}, nil
}

func findGoFile(ctx context.Context, arg fileFinder) (*vcsv1.GetFileResponse, error) {
	std, err := findGoStdFile(ctx, arg)
	if err != nil {
		return nil, err
	}
	if std != nil {
		return std, nil
	}

	deps, err := findGoDependencyFile(ctx, arg)
	if err != nil {
		return nil, err
	}
	if deps != nil {
		return deps, nil
	}
	// Try to find the file in the repo.
	path := strings.TrimPrefix(arg.path, strings.Join([]string{arg.repo.GetHostName(), arg.repo.GetOwnerName(), arg.repo.GetRepoName()}, "/"))
	path = strings.TrimLeft(path, "/")
	for {
		content, err := arg.client.GetFile(ctx, arg.repo.GetOwnerName(), arg.repo.GetRepoName(), path, arg.ref)
		if err != nil && errors.Is(err, ErrNotFound) {
			i := strings.Index(path, "/")
			if i < 0 {
				return nil, err
			}
			// remove the first path segment
			path = path[i+1:]
			continue
		}
		if err != nil {
			return nil, err
		}
		return newFileResponse(content.Content, content.URL)

	}
}

var versionSuffixRE = regexp.MustCompile(`/v[0-9]+[/]*`)

func findGoDependencyFile(ctx context.Context, arg fileFinder) (*vcsv1.GetFileResponse, error) {
	// if the path contains /vendor/ we can assume it's a dependency and it's versioned in the repo.
	vIdx := strings.Index(arg.path, "/vendor/")
	if vIdx > 0 {
		relativePath := strings.TrimPrefix(arg.path[vIdx:], "/")
		content, err := arg.client.GetFile(ctx, arg.repo.GetOwnerName(), arg.repo.GetRepoName(), relativePath, arg.ref)
		if err != nil {
			return nil, err
		}
		return newFileResponse(content.Content, content.URL)
	}
	mod, ok := parseGoModuleFilePath(arg.path)
	if !ok {
		return nil, nil
	}
	var err error
	mainModule := module.Version{
		Path:    filepath.Join(arg.repo.GetHostName(), arg.repo.GetOwnerName(), arg.repo.GetRepoName()),
		Version: module.PseudoVersion("", "", time.Time{}, arg.ref),
	}
	// we found a go module dependency
	modf, err := fetchGoMod(ctx, arg)
	if err != nil {
		level.Warn(arg.logger).Log("msg", "failed to fetch go.mod file", "err", err)
	} else {
		mainModule.Path = modf.Module.Mod.Path
	}
	// process go.mod file to find/correct the dependency version.
	mod = applyGoModule(modf, mod)
	mod, err = resolveVanityGoModule(ctx, mod, http.DefaultClient)
	if err != nil {
		return nil, fmt.Errorf("failed resolving vanity URL: %w", err)
	}
	mod = resolveLocalFile(mod, mainModule)
	// remove the version suffix
	mod.Path = versionSuffixRE.ReplaceAllString(mod.Path, "")
	return fetchGoDependencyFile(ctx, mod, arg.client)
}

func fetchGoDependencyFile(ctx context.Context, module goModuleFile, client VCSClient) (*vcsv1.GetFileResponse, error) {
	switch {
	case strings.HasPrefix(module.Path, "github.com/"):
		return fetchGithubModuleFile(ctx, module, client)
	case strings.HasPrefix(module.Path, "go.googlesource.com/"):
		return fetchGoogleSourceDependencyFile(ctx, module, http.DefaultClient)
	}
	return nil, nil
}

func fetchGithubModuleFile(ctx context.Context, mod goModuleFile, client VCSClient) (*vcsv1.GetFileResponse, error) {
	// todo: what if this is not a github repo?
	// 		VSClient should support querying multiple repo providers.
	githubFile, err := parseGithubRepo(mod)
	if err != nil {
		return nil, err
	}
	content, err := client.GetFile(ctx, githubFile.Owner, githubFile.Repo, githubFile.Path, githubFile.Ref)
	if err != nil {
		return nil, err
	}
	return newFileResponse(content.Content, content.URL)
}

type GitHubFile struct {
	Owner, Repo, Ref, Path string
}

func parseGithubRepo(mod goModuleFile) (GitHubFile, error) {
	if !strings.HasPrefix(mod.Path, "github.com/") {
		return GitHubFile{}, fmt.Errorf("invalid github URL: %s", mod.Path)
	}
	version, err := refFromVersion(mod.Version.Version)
	if err != nil {
		return GitHubFile{}, err
	}
	if version == "" {
		version = "main"
	}
	parts := strings.Split(mod.Path, "/")
	if len(parts) < 3 {
		return GitHubFile{}, fmt.Errorf("invalid github URL: %s", mod.Path)
	}
	return GitHubFile{
		Owner: parts[1],
		Repo:  parts[2],
		Ref:   version,
		Path:  filepath.Join(strings.Join(parts[3:], "/"), mod.filePath),
	}, nil
}

func refFromVersion(version string) (string, error) {
	if module.IsPseudoVersion(version) {
		rev, err := module.PseudoVersionRev(version)
		if err != nil {
			return "", err
		}
		return rev, nil
	}
	if sem := semver.Canonical(version); sem != "" {
		return sem, nil
	}

	return version, nil
}

// curl https://go.googlesource.com/oauth2/+/4ce7bbb2ffdc6daed06e2ec28916fd08d96bc3ea/amazon/amazon.go
// curl https://go.googlesource.com/oauth2/+/4ce7bbb2f/amazon/amazon.go\?format\=TEXT
// curl https://go.googlesource.com/oauth2/+/v0.16.0/amazon/amazon.go\?format\=TEXT
func fetchGoogleSourceDependencyFile(ctx context.Context, mod goModuleFile, httpClient *http.Client) (*vcsv1.GetFileResponse, error) {
	parts := strings.Split(strings.Trim(mod.Path, "/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid google source path: %s", mod.Path)
	}
	projectName := parts[1]
	filePath := mod.filePath
	extraPath := strings.Join(parts[2:], "/")
	if extraPath != "" {
		filePath = filepath.Join(extraPath, filePath)
	}
	version, err := refFromVersion(mod.Version.Version)
	if err != nil {
		return nil, err
	}
	if version == "" {
		version = "master"
	}
	url := fmt.Sprintf("https://go.googlesource.com/%s/+/%s/%s?format=TEXT", projectName, version, filePath)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("failed to fetch go lib %s: %s", mod.Path, resp.Status))
	}
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	decoded, err := base64.StdEncoding.DecodeString(string(content))
	if err != nil {
		return nil, err
	}
	return newFileResponse(string(decoded), url)
}

func resolveLocalFile(module goModuleFile, mainModule module.Version) goModuleFile {
	if !strings.HasPrefix(module.Path, ".") {
		return module
	}
	module.Version.Version = mainModule.Version
	module.Path = filepath.Join(mainModule.Path, module.Path)
	return module
}

// normally go-import meta tag should be used to resolve vanity.
//
//	curl -v 'https://google.golang.org/protobuf?go-get=1'
//
// careful follow redirect see: curl -L -v 'connectrpc.com/connect?go-get=1'
// if go-source meta tag is present prefer it over go-import.
// see https://go.dev/ref/mod#vcs-find
func resolveVanityGoModule(ctx context.Context, module goModuleFile, httpClient *http.Client) (goModuleFile, error) {
	if strings.Contains(module.Path, "github.com/") {
		return module, nil
	}
	// see https://labix.org/gopkg.in
	// gopkg.in/pkg.v3      → github.com/go-pkg/pkg (branch/tag v3, v3.N, or v3.N.M)
	// gopkg.in/user/pkg.v3 → github.com/user/pkg   (branch/tag v3, v3.N, or v3.N.M)
	if strings.Contains(module.Path, "gopkg.in/") {
		parts := strings.Split(module.Path, "/")
		if len(parts) < 2 {
			return goModuleFile{}, fmt.Errorf("invalid gopkg.in path: %s", module.Path)
		}
		packageNameParts := strings.Split(parts[len(parts)-1], ".")
		if len(packageNameParts) < 2 || packageNameParts[0] == "" {
			return goModuleFile{}, fmt.Errorf("invalid gopkg.in path: %s", module.Path)
		}
		switch len(parts) {
		case 2:
			module.Path = fmt.Sprintf("github.com/go-%s/%s", packageNameParts[0], packageNameParts[0])
		case 3:
			module.Path = fmt.Sprintf("github.com/%s/%s", parts[1], packageNameParts[0])
		default:
			return goModuleFile{}, fmt.Errorf("invalid gopkg.in path: %s", module.Path)
		}
		return module, nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://%s?go-get=1", strings.TrimRight(module.Path, "/")), nil)
	if err != nil {
		return goModuleFile{}, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return goModuleFile{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return goModuleFile{}, connect.NewError(connect.CodeNotFound, fmt.Errorf("failed to fetch go lib %s: %s", module.Path, resp.Status))
	}

	// look for go-source meta tag first
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return goModuleFile{}, err
	}
	var found bool
	// <meta name="go-source" content="google.golang.org/protobuf https://github.com/protocolbuffers/protobuf-go https://github.com/protocolbuffers/protobuf-go/tree/master{/dir} https://github.com/protocolbuffers/protobuf-go/tree/master{/dir}/{file}#L{line}">
	// <meta name="go-source" content="gopkg.in/alecthomas/kingpin.v2 _ https://github.com/alecthomas/kingpin/tree/v2.4.0{/dir} https://github.com/alecthomas/kingpin/blob/v2.4.0{/dir}/{file}#L{line}">
	doc.Find("meta[name='go-source']").Each(func(i int, s *goquery.Selection) {
		content, ok := s.Attr("content")
		if !ok {
			return
		}
		content = cleanWhiteSpace(content)
		parts := strings.Split(content, " ")
		if len(parts) < 2 {
			return
		}

		// prefer github if available in go-source
		if !found && strings.Contains(module.Path, parts[0]) && strings.Contains(parts[1], "github.com/") {
			found = true
			subPath := strings.Replace(module.Path, parts[0], "", 1)
			module.Path = filepath.Join(strings.TrimRight(
				strings.TrimPrefix(
					strings.TrimPrefix(parts[1], "https://"),
					"http://",
				), "/"),
				subPath,
			)

		}
	})
	if found {
		return module, nil
	}
	// <meta name="go-import" content="google.golang.org/protobuf git https://go.googlesource.com/protobuf">
	// <meta name="go-import" content="golang.org/x/oauth2 git https://go.googlesource.com/oauth2">
	// <meta name="go-import" content="go.uber.org/atomic git https://github.com/uber-go/atomic">
	doc.Find("meta[name='go-import']").Each(func(i int, s *goquery.Selection) {
		content, ok := s.Attr("content")
		if !ok {
			return
		}
		parts := strings.Split(cleanWhiteSpace(content), " ")
		if len(parts) < 3 {
			return
		}

		if !found && strings.Contains(module.Path, parts[0]) && parts[1] == "git" {
			found = true
			subPath := strings.Replace(module.Path, parts[0], "", 1)
			module.Path = filepath.Join(strings.TrimRight(
				strings.TrimPrefix(
					strings.TrimPrefix(parts[2], "https://"),
					"http://",
				), "/"),
				subPath,
			)

		}
	})
	return module, nil
}

func cleanWhiteSpace(s string) string {
	space := false
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return -1
		}
		if r == ' ' && space {
			return -1
		}
		space = r == ' '
		return r
	}, s)
}

// todo should move to golang package. and add tests
func applyGoModule(modf *modfile.File, module goModuleFile) goModuleFile {
	if modf == nil {
		return module
	}
	for _, req := range modf.Require {
		if req.Mod.Path == module.Path {
			module.Version.Version = req.Mod.Version
		}
	}
	for _, req := range modf.Replace {
		if req.Old.Path == module.Path {
			module.Path = req.New.Path
			module.Version.Version = req.New.Version
		}
	}
	return module
}

type goModuleFile struct {
	module.Version
	filePath string
}

func (m goModuleFile) String() string {
	return fmt.Sprintf("%s@%s", m.Path, m.Version)
}

func parseGoModuleFilePath(path string) (goModuleFile, bool) {
	parts := strings.Split(path, "@v")
	if len(parts) != 2 {
		return goModuleFile{}, false
	}
	first := strings.Index(parts[1], "/")
	if first < 0 {
		return goModuleFile{}, false
	}
	filePath := parts[1][first+1:]
	modulePath := parts[0]
	// searching for the first domain name
	domainParts := strings.Split(modulePath, "/")
	for i, part := range domainParts {
		if strings.Contains(part, ".") {
			return goModuleFile{
				Version: module.Version{
					Path:    strings.Join(domainParts[i:], "/"),
					Version: "v" + parts[1][:first],
				},
				filePath: filePath,
			}, true
		}
	}
	return goModuleFile{}, false
}

func fetchGoMod(ctx context.Context, arg fileFinder) (*modfile.File, error) {
	content, err := arg.client.GetFile(ctx, arg.repo.GetOwnerName(), arg.repo.GetRepoName(), "go.mod", arg.ref)
	if err != nil {
		return nil, err
	}
	return modfile.Parse("go.mod", []byte(content.Content), nil)
}

// findGoStdFile returns the file content if the given path is a standard go package.
func findGoStdFile(ctx context.Context, arg fileFinder) (*vcsv1.GetFileResponse, error) {
	if len(arg.path) == 0 {
		return nil, nil
	}
	path := strings.TrimSuffix(arg.path, "/usr/local/go/src/")
	path = strings.TrimSuffix(path, "$GOROOT/src/")
	fileName := filepath.Base(arg.path)
	packageName := strings.TrimSuffix(path, "/"+fileName)

	// Todo: Send more metadata from SDK to fetch the correct version of Go std packages.
	// For this we should use arbitrary k/v metadata in our request so that we don't need to change the API.
	// I thought about using go.mod go version but it's a min and doesn't guarantee it hasn't been built with a higher version.
	// Alternatively we could interpret the build system and use the version of the go compiler.
	ref := "master"
	isVendor := strings.HasPrefix(packageName, "vendor/")

	if _, isStd := golang.StandardPackages[packageName]; !isVendor && !isStd {
		return nil, nil
	}
	return fetchGoStd(ctx, path, ref)
}

// todo should move to golang package.
func fetchGoStd(ctx context.Context, path, ref string) (*vcsv1.GetFileResponse, error) {
	// https://raw.githubusercontent.com/golang/go/master/src/archive/tar/format.go
	url := fmt.Sprintf(`https://raw.githubusercontent.com/golang/go/%s/src/%s`, ref, path)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req) // todo: use a custom client with timeout
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("failed to fetch go lib %s: %s", url, resp.Status))
	}
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return newFileResponse(string(content), url)
}
