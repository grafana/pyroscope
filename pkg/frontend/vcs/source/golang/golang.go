package golang

import (
	"path/filepath"
	"strings"

	"github.com/grafana/regexp"
)

const (
	vendorPath = "vendor/"
	stdLocal   = "/usr/local/go/src/"
	stdGoRoot  = "$GOROOT/src/"
)

var (
	stdLibRegex        = regexp.MustCompile(`.*?\/go\/.*?(?P<version>(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?).*?\/src\/(?P<path>.*)`)
	toolchainGoVersion = regexp.MustCompile(`-go(?P<version>[0-9]+\.[0-9]+(?:\.[0-9]+)?(?:rc[0-9]+|beta[0-9]+)?)(?:\.|-)`)
)

// IsStandardLibraryPath returns the cleaned path of the standard library package and a potential version if detected
func IsStandardLibraryPath(path string) (string, string, bool) {
	if len(path) == 0 {
		return "", "", false
	}

	// match toolchain go mod paths
	if modFile, ok := ParseModuleFromPath(path); ok && modFile.Path == "golang.org/toolchain" {
		// figure out version
		matches := toolchainGoVersion.FindStringSubmatch(modFile.Version.Version)
		if len(matches) > 1 {
			return strings.TrimPrefix(modFile.FilePath, "src/"), matches[1], true
		}
	}

	if stdLibRegex.MatchString(path) {
		matches := stdLibRegex.FindStringSubmatch(path)
		version := matches[stdLibRegex.SubexpIndex("version")]
		path = matches[stdLibRegex.SubexpIndex("path")]
		return path, version, true
	}

	path = strings.TrimPrefix(path, stdLocal)
	path = strings.TrimPrefix(path, stdGoRoot)
	fileName := filepath.Base(path)
	packageName := strings.TrimSuffix(path, "/"+fileName)
	isStdVendor := strings.HasPrefix(packageName, vendorPath)

	if _, isStd := StandardPackages[packageName]; !isStdVendor && !isStd {
		return "", "", false
	}
	return path, "", true
}

// VendorRelativePath returns the relative path of the given path
// if it is a vendor path.
// For example:
// /drone/src/vendor/google.golang.org/protobuf/proto/merge.go -> /vendor/google.golang.org/protobuf/proto/merge.go
func VendorRelativePath(path string) (string, bool) {
	idx := strings.Index(path, "/"+vendorPath)
	if idx < 0 {
		return "", false
	}
	return path[idx:], true
}
