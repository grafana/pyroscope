// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/api/handlers.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package api

import (
	_ "embed"
	"html/template"
	"net/http"
	"path"
	"strings"

	"github.com/grafana/dskit/kv/memberlist"
)

//go:embed memberlist_status.gohtml
var memberlistStatusPageHTML string

func MemberlistStatusHandler(httpPathPrefix string, kvs *memberlist.KVInitService) http.Handler {
	templ := template.New("memberlist_status")
	templ.Funcs(map[string]interface{}{
		"AddPathPrefix": func(link string) string { return path.Join(httpPathPrefix, link) },
		"StringsJoin":   strings.Join,
	})
	template.Must(templ.Parse(memberlistStatusPageHTML))
	return memberlist.NewHTTPStatusHandler(kvs, templ)
}
