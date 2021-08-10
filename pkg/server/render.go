package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
)

var (
	errUnknownFormat         = errors.New("unknown format")
	errLabelIsRequired       = errors.New("label parameter is required")
	errNoData                = errors.New("no data")
	errTimeParamsAreRequired = errors.New("leftFrom,leftUntil,rightFrom,rightUntil are required")
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
	if ok := ctrl.expectJSON(w, p.format); !ok {
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

	fs := out.Tree.FlamebearerStruct(p.maxNodes)
	res := renderResponse(fs, out)
	ctrl.writeResponseJSON(w, res)
}

func (ctrl *Controller) renderDiffHandler(w http.ResponseWriter, r *http.Request) {
	var p renderParams
	if err := ctrl.renderParametersFromRequest(r, &p); err != nil {
		ctrl.writeInvalidParameterError(w, err)
		return
	}
	if ok := ctrl.expectJSON(w, p.format); !ok {
		return
	}

	leftStartTime, leftEndTime, leftOK := parseRenderRangeParams(r.URL.Query(), "leftFrom", "leftUntil")
	rghtStartTime, rghtEndTime, rghtOK := parseRenderRangeParams(r.URL.Query(), "rightFrom", "rightUntil")
	if !leftOK || !rghtOK {
		ctrl.writeInvalidParameterError(w, errTimeParamsAreRequired)
		return
	}

	out, leftOut, rghtOut, err := ctrl.loadTreeConcurrently(p.gi, p.gi.StartTime, p.gi.EndTime, leftStartTime, leftEndTime, rghtStartTime, rghtEndTime)
	if err != nil {
		ctrl.writeInternalServerError(w, err, "failed to retrieve data")
		return
	}
	// TODO: handle properly, see ctrl.renderHandler
	if out == nil {
		ctrl.writeInternalServerError(w, errNoData, "failed to retrieve data")
		return
	}

	leftOut.Tree, rghtOut.Tree = tree.CombineTree(leftOut.Tree, rghtOut.Tree)
	fs := tree.CombineToFlamebearerStruct(leftOut.Tree, rghtOut.Tree, p.maxNodes)
	res := renderResponse(fs, out)
	ctrl.writeResponseJSON(w, res)
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

func renderResponse(fs *tree.Flamebearer, out *storage.GetOutput) map[string]interface{} {
	// TODO remove this duplication? We're already adding this to metadata
	fs.SpyName = out.SpyName
	fs.SampleRate = out.SampleRate
	fs.Units = out.Units
	res := map[string]interface{}{
		"timeline":    out.Timeline,
		"flamebearer": fs,
		"metadata": map[string]interface{}{
			"format":     fs.Format, // "single" | "double"
			"spyName":    out.SpyName,
			"sampleRate": out.SampleRate,
			"units":      out.Units,
		},
	}
	return res
}

func parseRenderRangeParams(v url.Values, from, until string) (startTime, endTime time.Time, ok bool) {
	fromStr, untilStr := v.Get(from), v.Get(until)
	startTime, endTime = attime.Parse(fromStr), attime.Parse(untilStr)
	return startTime, endTime, fromStr != "" || untilStr != ""
}

func (ctrl *Controller) loadTreeConcurrently(
	gi *storage.GetInput,
	treeStartTime, treeEndTime time.Time,
	leftStartTime, leftEndTime time.Time,
	rghtStartTime, rghtEndTime time.Time,
) (treeOut, leftOut, rghtOut *storage.GetOutput, _ error) {

	var treeErr, leftErr, rghtErr error
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); treeOut, treeErr = ctrl.loadTree(gi, treeStartTime, treeEndTime) }()
	go func() { defer wg.Done(); leftOut, leftErr = ctrl.loadTree(gi, leftStartTime, leftEndTime) }()
	go func() { defer wg.Done(); rghtOut, rghtErr = ctrl.loadTree(gi, rghtStartTime, rghtEndTime) }()
	wg.Wait()

	for _, err := range []error{treeErr, leftErr, rghtErr} {
		if err != nil {
			return nil, nil, nil, err
		}
	}
	return treeOut, leftOut, rghtOut, nil
}

func (ctrl *Controller) loadTree(gi *storage.GetInput, startTime, endTime time.Time) (_ *storage.GetOutput, _err error) {
	defer func() {
		rerr := recover()
		if rerr != nil {
			_err = fmt.Errorf("panic: %v", rerr)
			ctrl.log.WithFields(logrus.Fields{
				"recover": rerr,
				"stack":   string(debug.Stack()),
			}).Error("loadTree: recovered from panic")
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
