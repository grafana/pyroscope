package golang

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/grafana/regexp"
)

const (
	vendorPath = "vendor/"
	stdLocal   = "/usr/local/go/src/"
	stdGoRoot  = "$GOROOT/src/"
)

var stdLibRegex = regexp.MustCompile(`.*?\/go\/.*?(?P<version>(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?).*?\/src\/(?P<path>.*)`)

// StandardLibraryURL returns the URL of the standard library package
// from the given local path if it exists.
func StandardLibraryURL(path string) (string, bool) {
	if len(path) == 0 {
		return "", false
	}

	if stdLibRegex.MatchString(path) {
		matches := stdLibRegex.FindStringSubmatch(path)
		version := matches[stdLibRegex.SubexpIndex("version")]
		path = matches[stdLibRegex.SubexpIndex("path")]
		return fmt.Sprintf(`https://raw.githubusercontent.com/golang/go/go%s/src/%s`, version, path), true
	}

	path = strings.TrimPrefix(path, stdLocal)
	path = strings.TrimPrefix(path, stdGoRoot)
	fileName := filepath.Base(path)
	packageName := strings.TrimSuffix(path, "/"+fileName)
	// Todo: Send more metadata from SDK to fetch the correct version of Go std packages.
	// For this we should use arbitrary k/v metadata in our request so that we don't need to change the API.
	// I thought about using go.mod go version but it's a min and doesn't guarantee it hasn't been built with a higher version.
	// Alternatively we could interpret the build system and use the version of the go compiler.
	ref := "master"
	isStdVendor := strings.HasPrefix(packageName, vendorPath)

	if _, isStd := StandardPackages[packageName]; !isStdVendor && !isStd {
		return "", false
	}
	return fmt.Sprintf(`https://raw.githubusercontent.com/golang/go/%s/src/%s`, ref, path), true
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
