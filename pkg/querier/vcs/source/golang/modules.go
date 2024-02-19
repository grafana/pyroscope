package golang

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"connectrpc.com/connect"
	"github.com/PuerkitoBio/goquery"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

const (
	GoMod = "go.mod"

	GitHubPath  = "github.com/"
	GooglePath  = "go.googlesource.com/"
	GoPkgInPath = "gopkg.in/"
)

var versionSuffixRE = regexp.MustCompile(`/v[0-9]+[/]*`)

// Module represents a go module with a file path in that module
type Module struct {
	module.Version
	FilePath string
}

// ParseModuleFromPath parses the module from the given path.
func ParseModuleFromPath(path string) (Module, bool) {
	parts := strings.Split(path, "@v")
	if len(parts) != 2 {
		return Module{}, false
	}
	first := strings.Index(parts[1], "/")
	if first < 0 {
		return Module{}, false
	}
	filePath := parts[1][first+1:]
	modulePath := parts[0]
	// searching for the first domain name
	domainParts := strings.Split(modulePath, "/")
	for i, part := range domainParts {
		if strings.Contains(part, ".") {
			return Module{
				Version: module.Version{
					Path:    strings.Join(domainParts[i:], "/"),
					Version: "v" + parts[1][:first],
				},
				FilePath: filePath,
			}, true
		}
	}
	return Module{}, false
}

func (m Module) IsGitHub() bool {
	return strings.HasPrefix(m.Path, GitHubPath)
}

func (m Module) IsGoogleSource() bool {
	return strings.HasPrefix(m.Path, GooglePath)
}

func (m Module) IsGoPkgIn() bool {
	return strings.HasPrefix(m.Path, GoPkgInPath)
}

func (m Module) String() string {
	return fmt.Sprintf("%s@%s", m.Path, m.Version)
}

type HttpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Resolve resolves the module path to a canonical path.
func (module *Module) Resolve(ctx context.Context, mainModule module.Version, modfile *modfile.File, httpClient HttpClient) error {
	if modfile != nil {
		mainModule.Path = modfile.Module.Mod.Path
		module.applyGoMod(mainModule, modfile)
	}
	if err := module.resolveVanityURL(ctx, httpClient); err != nil {
		return err
	}
	// remove version suffix such as /v2 or /v11 ...
	module.Path = versionSuffixRE.ReplaceAllString(module.Path, "")
	return nil
}

func (module *Module) resolveVanityURL(ctx context.Context, httpClient HttpClient) error {
	switch {
	// no need to resolve vanity URL
	case module.IsGitHub():
		return nil
	case module.IsGoPkgIn():
		return module.resolveGoPkgIn()
	default:
		return module.resolveGoGet(ctx, httpClient)
	}
}

// resolveGoGet resolves the module path using go-get meta tags.
// normally go-import meta tag should be used to resolve vanity.
//
//	curl -v 'https://google.golang.org/protobuf?go-get=1'
//
// careful follow redirect see: curl -L -v 'connectrpc.com/connect?go-get=1'
// if go-source meta tag is present prefer it over go-import.
// see https://go.dev/ref/mod#vcs-find
func (module *Module) resolveGoGet(ctx context.Context, httpClient HttpClient) error {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://%s?go-get=1", strings.TrimRight(module.Path, "/")), nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("failed to fetch go lib %s: %s", module.Path, resp.Status))
	}

	// look for go-source meta tag first
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return err
	}
	var found bool
	// <meta name="go-source" content="google.golang.org/protobuf https://github.com/protocolbuffers/protobuf-go https://github.com/protocolbuffers/protobuf-go/tree/master{/dir} https://github.com/protocolbuffers/protobuf-go/tree/master{/dir}/{file}#L{line}">
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
		return nil
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
	return nil
}

// resolveGoPkgIn resolves the gopkg.in path to a github path.
// see https://labix.org/gopkg.in
// gopkg.in/pkg.v3      → github.com/go-pkg/pkg (branch/tag v3, v3.N, or v3.N.M)
// gopkg.in/user/pkg.v3 → github.com/user/pkg   (branch/tag v3, v3.N, or v3.N.M)
func (module *Module) resolveGoPkgIn() error {
	parts := strings.Split(module.Path, "/")
	if len(parts) < 2 {
		return fmt.Errorf("invalid gopkg.in path: %s", module.Path)
	}
	packageNameParts := strings.Split(parts[len(parts)-1], ".")
	if len(packageNameParts) < 2 || packageNameParts[0] == "" {
		return fmt.Errorf("invalid gopkg.in path: %s", module.Path)
	}
	switch len(parts) {
	case 2:
		module.Path = fmt.Sprintf("github.com/go-%s/%s", packageNameParts[0], packageNameParts[0])
	case 3:
		module.Path = fmt.Sprintf("github.com/%s/%s", parts[1], packageNameParts[0])
	default:
		return fmt.Errorf("invalid gopkg.in path: %s", module.Path)
	}
	return nil
}

// applyGoMod applies the go.mod file to the module.
func (module *Module) applyGoMod(mainModule module.Version, modf *modfile.File) {
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
	if strings.HasPrefix(module.Path, "./") {
		module.Version.Version = mainModule.Version
		module.Path = filepath.Join(mainModule.Path, module.Path)
	}
}

type GitHubFile struct {
	Owner, Repo, Ref, Path string
}

// GithubFile returns the github file information.
func (m Module) GithubFile() (GitHubFile, error) {
	if !m.IsGitHub() {
		return GitHubFile{}, fmt.Errorf("invalid github URL: %s", m.Path)
	}
	version, err := refFromVersion(m.Version.Version)
	if err != nil {
		return GitHubFile{}, err
	}
	if version == "" {
		version = "main"
	}
	parts := strings.Split(m.Path, "/")
	if len(parts) < 3 {
		return GitHubFile{}, fmt.Errorf("invalid github URL: %s", m.Path)
	}
	return GitHubFile{
		// ! character is used for capitalization
		// example: github.com/!f!zambia/eagle@v0.0.2/eagle.go
		Owner: strings.ReplaceAll(parts[1], "!", ""),
		Repo:  parts[2],
		Ref:   version,
		Path:  filepath.Join(strings.Join(parts[3:], "/"), m.FilePath),
	}, nil
}

// GoogleSourceURL returns the URL of the file in the google source repository.
// Example https://go.googlesource.com/oauth2/+/4ce7bbb2ffdc6daed06e2ec28916fd08d96bc3ea/amazon/amazon.go
func (m Module) GoogleSourceURL() (string, error) {
	if !m.IsGoogleSource() {
		return "", fmt.Errorf("invalid google source path: %s", m.Path)
	}
	parts := strings.Split(strings.Trim(m.Path, "/"), "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid google source path: %s", m.Path)
	}
	projectName := parts[1]
	filePath := m.FilePath
	extraPath := strings.Join(parts[2:], "/")
	if extraPath != "" {
		filePath = filepath.Join(extraPath, filePath)
	}
	version, err := refFromVersion(m.Version.Version)
	if err != nil {
		return "", err
	}
	if version == "" {
		version = "master"
	}
	return fmt.Sprintf("https://go.googlesource.com/%s/+/%s/%s?format=TEXT", projectName, version, filePath), nil
}

// refFromVersion returns the git ref from the given module version.
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

// cleanWhiteSpace removes all white space characters from the given string.
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
