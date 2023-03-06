// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/api/handlers.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package api

import (
	"embed"
	"html/template"
	"net/http"
	"path"
	"sort"
	"sync"
)

// List of weights to order link groups in the same order as weights are ordered here.
const (
	serviceStatusWeight = iota
	configWeight
	RuntimeConfigWeight
	DefaultWeight
	memberlistWeight
	dangerousWeight
	OpenAPIDefinitionWeight
)

func NewIndexPageContent() *IndexPageContent {
	return &IndexPageContent{}
}

// IndexPageContent is a map of sections to path -> description.
type IndexPageContent struct {
	mu sync.Mutex

	elements []IndexPageLinkGroup
}

type IndexPageLinkGroup struct {
	weight int
	Desc   string
	Links  []IndexPageLink
}

type IndexPageLink struct {
	Desc      string
	Path      string
	Dangerous bool
}

func (pc *IndexPageContent) AddLinks(weight int, groupDesc string, links []IndexPageLink) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.elements = append(pc.elements, IndexPageLinkGroup{weight: weight, Desc: groupDesc, Links: links})
}

func (pc *IndexPageContent) GetContent() []IndexPageLinkGroup {
	pc.mu.Lock()
	els := append([]IndexPageLinkGroup(nil), pc.elements...)
	pc.mu.Unlock()

	sort.Slice(els, func(i, j int) bool {
		if els[i].weight != els[j].weight {
			return els[i].weight < els[j].weight
		}
		return els[i].Desc < els[j].Desc
	})

	return els
}

//go:embed index.gohtml
var indexPageHTML string

type indexPageContents struct {
	LinkGroups []IndexPageLinkGroup
}

//go:embed static
var StaticFiles embed.FS

func IndexHandler(httpPathPrefix string, content *IndexPageContent) http.HandlerFunc {
	templ := template.New("main")
	templ.Funcs(map[string]interface{}{
		"AddPathPrefix": func(link string) string {
			return path.Join(httpPathPrefix, link)
		},
	})
	template.Must(templ.Parse(indexPageHTML))

	return func(w http.ResponseWriter, r *http.Request) {
		err := templ.Execute(w, indexPageContents{LinkGroups: content.GetContent()})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
