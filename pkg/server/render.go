package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
)

type RenderFormat string

const (
	FormatSingle = "single"
	FormatDouble = "double"
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

	var out *storage.GetOutput
	var fs *tree.Flamebearer
	var err error
	var format RenderFormat

	leftStartTime, leftEndTime, leftOK := parseRenderRangeParams(r.URL.Query(), "leftFrom", "leftUntil")
	rghtStartTime, rghtEndTime, rghtOK := parseRenderRangeParams(r.URL.Query(), "rightFrom", "rightUntil")

	if rghtOK || leftOK {
		var leftOut, rghtOut *storage.GetOutput
		leftOut, rghtOut, err = ctrl.loadDiffOutput(p.gi, leftStartTime, leftEndTime, rghtStartTime, rghtEndTime)
		ctrl.statsInc("render")
		if err != nil {
			ctrl.writeInternalServerError(w, err, "failed to retrieve data")
			return
		}
		out = leftOut // to be compatible with responding code
		fs = tree.CombineToFlamebearerStruct(leftOut.Tree, rghtOut.Tree, p.maxNodes)
		format = FormatDouble

	} else {
		out, err = ctrl.storage.Get(p.gi)
		ctrl.statsInc("render")
		if err != nil {
			ctrl.writeInternalServerError(w, err, "failed to retrieve data")
			return
		}

		// TODO: handle properly
		if out == nil {
			out = &storage.GetOutput{Tree: tree.New()}
		}
		fs = out.Tree.FlamebearerStruct(p.maxNodes)
		format = FormatSingle
	}

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
			"format":     format, // "single" | "double"
		},
	}

	w.Header().Set("Content-Type", "application/json")
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

func parseRenderRangeParams(v url.Values, from, until string) (startTime, endTime time.Time, ok bool) {
	fromStr, untilStr := v.Get(from), v.Get(until)
	startTime, endTime = attime.Parse(fromStr), attime.Parse(untilStr)
	return startTime, endTime, fromStr != "" || untilStr != ""
}

func (ctrl *Controller) loadDiffOutput(
	gi *storage.GetInput,
	leftStartTime, leftEndTime time.Time,
	rghtStartTime, rghtEndTime time.Time,
) (*storage.GetOutput, *storage.GetOutput, error) {

	var leftOut, rghtOut *storage.GetOutput
	var leftErr, rghtErr error

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); leftOut, leftErr = ctrl.loadDiffOutputExec(gi, leftStartTime, leftEndTime) }()
	go func() { defer wg.Done(); rghtOut, rghtErr = ctrl.loadDiffOutputExec(gi, rghtStartTime, rghtEndTime) }()
	wg.Wait()

	if leftErr != nil {
		return nil, nil, leftErr
	}
	if rghtErr != nil {
		return nil, nil, rghtErr
	}
	leftOut.Tree, rghtOut.Tree = combineTree(leftOut.Tree, rghtOut.Tree)
	return leftOut, rghtOut, nil
}

func (ctrl *Controller) loadDiffOutputExec(gi *storage.GetInput, startTime, endTime time.Time) (_ *storage.GetOutput, _err error) {
	defer func() {
		rerr := recover()
		if rerr != nil {
			_err = fmt.Errorf("panic: %v", rerr)
		}
	}()

	_gi := *gi // clone the struct
	_gi.StartTime, _gi.EndTime = startTime, endTime
	out, err := ctrl.storage.Get(&_gi)
	if err != nil {
		return nil, err
	}
	if out == nil {
		// TODO: handle properly
		return &storage.GetOutput{Tree: tree.New()}, nil
	}
	return out, nil
}

func combineTree(leftTree, rightTree *tree.Tree) (*tree.Tree, *tree.Tree) {
	return tree.CombineTree(leftTree, rightTree)
}
