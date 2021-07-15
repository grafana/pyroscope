package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/pyroscope-io/pyroscope/pkg/pyroql"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
)

var (
	errUnknownFormat   = errors.New("unknown format")
	errLabelIsRequired = errors.New("label parameter is required")
)

func (ctrl *Controller) renderHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var gi storage.GetInput
	if err := resolveGetParams(q, &gi); err != nil {
		ctrl.writeInvalidParameterError(w, err)
		return
	}

	switch q.Get("format") {
	case "json", "":
	default:
		ctrl.writeInvalidParameterError(w, errUnknownFormat)
		return
	}

	out, err := ctrl.storage.Get(&gi)
	ctrl.statsInc("render")
	if err != nil {
		ctrl.writeInternalServerError(w, err, "failed to retrieve data")
		return
	}

	// TODO: handle properly
	if out == nil {
		out = &storage.GetOutput{Tree: tree.New()}
	}

	maxNodes := ctrl.config.MaxNodesRender
	if mn, err := strconv.Atoi(q.Get("max-nodes")); err == nil && mn > 0 {
		maxNodes = mn
	}

	w.Header().Set("Content-Type", "application/json")
	fs := out.Tree.FlamebearerStruct(maxNodes)
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

func resolveGetParams(v url.Values, gi *storage.GetInput) error {
	gi.StartTime = attime.Parse(v.Get("from"))
	gi.EndTime = attime.Parse(v.Get("until"))
	k := v.Get("name")
	if k != "" {
		sk, err := storage.ParseKey(k)
		if err != nil {
			return fmt.Errorf("name: parsing storage key: %w", err)
		}
		gi.Key = sk
		return nil
	}
	q := v.Get("query")
	if q == "" {
		return fmt.Errorf("'query' or 'name' parameter is required")
	}
	qry, err := pyroql.ParseQuery(q)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	gi.Query = qry
	return nil
}
