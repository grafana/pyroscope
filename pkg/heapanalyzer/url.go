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
