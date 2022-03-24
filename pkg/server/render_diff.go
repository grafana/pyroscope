package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
)

// RenderDiffParams refers to the params accepted by the renderDiffHandler
type RenderDiffParams struct {
	LeftQuery string `json:"leftQuery"`
	LeftFrom  string `json:"leftFrom"`
	LeftUntil string `json:"leftUntil"`

	RightQuery string `json:"rightQuery"`
	RightFrom  string `json:"rightFrom"`
	RightUntil string `json:"rightUntil"`

	Format   string `json:"format"`
	MaxNodes *int   `json:"maxNodes,omitempty"`
}

// RenderDiffResponse refers to the response of the renderDiffHandler
type RenderDiffResponse struct {
	*flamebearer.FlamebearerProfile
}

type diffParams struct {
	Left  storage.GetInput
	Right storage.GetInput

	Format   string
	MaxNodes int
}

// parseDiffQueryParams parses query params into a diffParams
func (ctrl *Controller) parseDiffQueryParams(r *http.Request, p *diffParams) (err error) {
	parseDiffQueryParams := func(r *http.Request, prefix string) (gi storage.GetInput, err error) {
		v := r.URL.Query()
		getWithPrefix := func(param string) string {
			return v.Get(prefix + strings.Title(param))
		}

		// Parse query
		qry, err := flameql.ParseQuery(getWithPrefix("query"))
		if err != nil {
			return gi, fmt.Errorf("%q: %+w", "Error parsing query", err)
		}
		gi.Query = qry

		gi.StartTime = attime.Parse(getWithPrefix("from"))
		gi.EndTime = attime.Parse(getWithPrefix("until"))

		return gi, nil
	}

	p.Left, err = parseDiffQueryParams(r, "left")
	if err != nil {
		return fmt.Errorf("%q: %+w", "Could not parse 'left' side", err)
	}

	p.Right, err = parseDiffQueryParams(r, "right")
	if err != nil {
		return fmt.Errorf("%q: %+w", "Could not parse 'right' side", err)
	}

	// Parse the common fields
	v := r.URL.Query()
	p.MaxNodes = ctrl.config.MaxNodesRender
	if mn, err := strconv.Atoi(v.Get("max-nodes")); err == nil && mn > 0 {
		p.MaxNodes = mn
	}

	p.Format = v.Get("format")
	return ctrl.expectFormats(p.Format)
}

func (ctrl *Controller) renderDiffHandler(w http.ResponseWriter, r *http.Request) {
	var params diffParams

	switch r.Method {
	case http.MethodGet:
		if err := ctrl.parseDiffQueryParams(r, &params); err != nil {
			ctrl.writeInvalidParameterError(w, err)
			return
		}
	default:
		ctrl.writeInvalidMethodError(w)
		return
	}

	// Load Both trees
	// TODO: do this concurrently
	leftOut, err := ctrl.loadTree(&params.Left, params.Left.StartTime, params.Left.EndTime)
	if err != nil {
		ctrl.writeInvalidParameterError(w, fmt.Errorf("%q: %+w", "could not load 'left' tree", err))
		return
	}

	rightOut, err := ctrl.loadTree(&params.Right, params.Right.StartTime, params.Right.EndTime)
	if err != nil {
		ctrl.writeInvalidParameterError(w, fmt.Errorf("%q: %+w", "could not load 'right' tree", err))
		return
	}

	combined, err := flamebearer.NewCombinedProfile("diff", leftOut, rightOut, params.MaxNodes)
	if err != nil {
		ctrl.writeInvalidParameterError(w, err)
		return
	}

	switch params.Format {
	case "html":
		w.Header().Add("Content-Type", "text/html")
		if err := flamebearer.FlamebearerToStandaloneHTML(&combined, ctrl.dir, w); err != nil {
			ctrl.writeJSONEncodeError(w, err)
			return
		}

	case "json":
		// fallthrough to default, to maintain existing behaviour
		fallthrough
	default:
		res := RenderDiffResponse{&combined}
		ctrl.writeResponseJSON(w, res)
	}
}
