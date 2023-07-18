// SPDX-License-Identifier: AGPL-3.0-only

package storegateway

import (
	_ "embed" // Used to embed html template
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/grafana/mimir/pkg/util"
)

//go:embed tenants.gohtml
var tenantsPageHTML string
var tenantsTemplate = template.Must(template.New("webpage").Parse(tenantsPageHTML))

type tenantsPageContents struct {
	Now     time.Time `json:"now"`
	Tenants []string  `json:"tenants,omitempty"`
}

func (s *StoreGateway) TenantsHandler(w http.ResponseWriter, req *http.Request) {
	tenantIDs, err := s.stores.scanUsers(req.Context())
	if err != nil {
		util.WriteTextResponse(w, fmt.Sprintf("Can't read tenants: %s", err))
		return
	}

	util.RenderHTTPResponse(w, tenantsPageContents{
		Now:     time.Now(),
		Tenants: tenantIDs,
	}, tenantsTemplate, req)
}
