package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
)

var (
	errUnknownFormat   = errors.New("unknown format")
	errLabelIsRequired = errors.New("label parameter is required")
)

type renderParams struct {
	format   string
	maxNodes int
	gi       *storage.GetInput
}

func (ctrl *Controller) renderHandler(w http.ResponseWriter, r *http.Request) {
	var p renderParams
	if err := ctrl.renderParametersFromRequest(r, &p); err != nil {
		ctrl.writeInvalidParameterError(w, err)
		return
	}

	switch p.format {
	case "json", "":
	default:
		ctrl.writeInvalidParameterError(w, errUnknownFormat)
		return
	}

	out, err := ctrl.storage.Get(p.gi)
	ctrl.statsInc("render")
	if err != nil {
		ctrl.writeInternalServerError(w, err, "failed to retrieve data")
		return
	}

	// TODO: handle properly
	if out == nil {
		out = &storage.GetOutput{Tree: tree.New()}
	}

	w.Header().Set("Content-Type", "application/json")
	fs := out.Tree.FlamebearerStruct(p.maxNodes)
	// TODO remove this duplication? We're already adding this to metadata
	fs.SpyName = out.SpyName
	fs.SampleRate = out.SampleRate
	fs.Units = out.Units
	res := map[string]interface{}{
		"timeline":    out.Timeline,
		"flamebearer": fs,
		"metadata": map[string]interface{}{
			"spyName":    out.SpyName,
			"sampleRate": out.SampleRate,
			"units":      out.Units,
		},
	}

	if err = json.NewEncoder(w).Encode(res); err != nil {
		ctrl.writeJSONEncodeError(w, err)
	}
}

func (ctrl *Controller) renderParametersFromRequest(r *http.Request, p *renderParams) error {
	v := r.URL.Query()
	p.gi = new(storage.GetInput)

	k := v.Get("name")
	q := v.Get("query")

	switch {
	case k == "" && q == "":
		return fmt.Errorf("'query' or 'name' parameter is required")
	case k != "":
		sk, err := segment.ParseKey(k)
		if err != nil {
			return fmt.Errorf("name: parsing storage key: %w", err)
		}
		p.gi.Key = sk
	case q != "":
		qry, err := flameql.ParseQuery(q)
		if err != nil {
			return fmt.Errorf("query: %w", err)
		}
		p.gi.Query = qry
	}

	p.maxNodes = ctrl.config.MaxNodesRender
	if mn, err := strconv.Atoi(v.Get("max-nodes")); err == nil && mn > 0 {
		p.maxNodes = mn
	}

	p.gi.StartTime = attime.Parse(v.Get("from"))
	p.gi.EndTime = attime.Parse(v.Get("until"))
	p.format = v.Get("format")
	return nil
}
