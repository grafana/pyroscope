//go:generate go run gen.go > packages_gen.go

package golang

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// StdPackages returns a map of all standard packages for the given version.
func StdPackages(version string) (map[string]struct{}, error) {
	if version != "" {
		version = "@go" + version
	}
	res, err := http.Get(fmt.Sprintf("https://pkg.go.dev/std%s", version))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		log.Fatalf("status code error: %d %s", res.StatusCode, res.Status)
	}
	// Load the HTML document
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, err
	}
	packages := map[string]struct{}{}
	doc.Find("tbody").Each(func(i int, s *goquery.Selection) {
		// For each item found, get the title
		s.Find("a").Each(func(i int, s *goquery.Selection) {
			href, ok := s.Attr("href")
			if !ok {
				return
			}
			// extract the package name from the href
			// /crypto/internal/alias@go1.21.6
			if len(strings.Split(href, "@")) > 1 {
				packages[strings.Trim(strings.Split(href, "@")[0], "/")] = struct{}{}
			}
		})
	})
	return packages, nil
}
