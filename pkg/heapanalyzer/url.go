package heapanalyzer

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var searchableType = regexp.MustCompile(`github\.com/([a-zA-Z0-9_-]+/[a-zA-Z0-9_-]+)/([a-zA-Z0-9_/-]*)?\.([a-zA-Z0-9_-]+)`)

// getCodeSearchUrl returns a url to search for the object type in the code
// for now it only supports github
func getCodeSearchUrl(t string) string {
	// for the case of simplicity we only support github
	if !strings.Contains(t, "github.com") {
		return ""
	}

	// hardcode for the demo
	if t == "github.com/aws/aws-sdk-go/aws/endpoints.endpoint" {
		return "https://github.com/aws/aws-sdk-go/blob/c314bba988044f1c8e47f85327898d4d9d4f72ac/aws/endpoints/defaults.go#L116"
	}

	matches := searchableType.FindStringSubmatch(t)
	if matches == nil {
		return ""
	}

	// support only happy path for now
	if len(matches) < 4 {
		return ""
	}

	q := fmt.Sprintf(`repo:%s path:/^%s/  %s`, matches[1], strings.ReplaceAll(matches[2]+`/`, "/", "\\/"), matches[3])
	return "https://github.com/search?q=" + url.QueryEscape(q) + "&type=code"
}
