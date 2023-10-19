// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/grafana/mimir/blob/main/pkg/compactor/compactor_http.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package compactor

import (
	_ "embed" // Used to embed html template
	"html/template"
	"net/http"

	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"

	util_log "github.com/grafana/pyroscope/pkg/util"
)

var (
	//go:embed status.gohtml
	statusPageHTML     string
	statusPageTemplate = template.Must(template.New("main").Parse(statusPageHTML))
)

type statusPageContents struct {
	Message string
}

func writeMessage(w http.ResponseWriter, message string) {
	w.WriteHeader(http.StatusOK)
	err := statusPageTemplate.Execute(w, statusPageContents{Message: message})
	if err != nil {
		level.Error(util_log.Logger).Log("msg", "unable to serve compactor ring page", "err", err)
	}
}

func (c *MultitenantCompactor) RingHandler(w http.ResponseWriter, req *http.Request) {
	if c.State() != services.Running {
		// we cannot read the ring before MultitenantCompactor is in Running state,
		// because that would lead to race condition.
		writeMessage(w, "Compactor is not running yet.")
		return
	}

	c.ring.ServeHTTP(w, req)
}
