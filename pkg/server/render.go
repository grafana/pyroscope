package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	errMethodNotAllowed      = errors.New("Method not allowed")
)

type renderParams struct {
	format   string
	maxNodes int
	gi       *storage.GetInput

	leftStartTime time.Time
	leftEndTime   time.Time
	rghtStartTime time.Time
	rghtEndTime   time.Time
}

func (ctrl *Controller) renderHandler(w http.ResponseWriter, r *http.Request) {
	var p renderParams
	if err := ctrl.renderParametersFromRequest(r, &p); err != nil {
		ctrl.writeInvalidParameterError(w, err)
		return
	}

	if err := ctrl.expectJSON(p.format); err != nil {
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

	fs := out.Tree.FlamebearerStruct(p.maxNodes)
	res := renderResponse(fs, out)
	ctrl.writeResponseJSON(w, res)
}

func (ctrl *Controller) renderDiffHandler(w http.ResponseWriter, r *http.Request) {

	var (
		p  renderParams
		rP RenderDiffParams

		leftStartParam string
		leftEndParam   string
		rghtStartParam string
		rghtEndParam   string
	)

	switch r.Method {
	case http.MethodGet:
		if err := ctrl.renderParametersFromRequest(r, &p); err != nil {
			ctrl.writeInvalidParameterError(w, err)
			return
		}
		leftStartParam, leftEndParam = "leftFrom", "leftUntil"
		rghtStartParam, rghtEndParam = "rightFrom", "rightUntil"

	case http.MethodPost:
		if err := ctrl.renderParametersFromRequestBody(r, &p, &rP); err != nil {
			ctrl.writeInvalidParameterError(w, err)
			return
		}
		leftStartParam, leftEndParam = rP.Left.From, rP.Left.Until
		rghtStartParam, rghtEndParam = rP.Right.From, rP.Right.Until

	default:
		ctrl.writeInvalidMethodError(w, errMethodNotAllowed)
		return
	}

	leftStartTime, leftEndTime, leftOK := parseRenderRangeParams(r, leftStartParam, leftEndParam)
	rghtStartTime, rghtEndTime, rghtOK := parseRenderRangeParams(r, rghtStartParam, rghtEndParam)
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

	if err := ctrl.expectJSON(p.format); err != nil {
		return err
	}

	return nil
}

func (ctrl *Controller) renderParametersFromRequestBody(r *http.Request, p *renderParams, rP *RenderDiffParams) error {

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(rP); err != nil {
		return err
	}

	p.gi = new(storage.GetInput)
	switch {
	case rP.Name == nil && rP.Query == nil:
		return fmt.Errorf("'query' or 'name' parameter is required")
	case rP.Name != nil:
		sk, err := segment.ParseKey(*rP.Name)
		if err != nil {
			return fmt.Errorf("name: parsing storage key: %w", err)
		}
		p.gi.Key = sk
	case rP.Query != nil:
		qry, err := flameql.ParseQuery(*rP.Query)
		if err != nil {
			return fmt.Errorf("query: %w", err)
		}
		p.gi.Query = qry
	}

	p.maxNodes = ctrl.config.MaxNodesRender
	if rP.MaxNodes != nil && *rP.MaxNodes > 0 {
		p.maxNodes = *rP.MaxNodes
	}

	p.gi.StartTime = attime.Parse(rP.From)
	p.gi.EndTime = attime.Parse(rP.Until)
	p.format = rP.Format

	if err := ctrl.expectJSON(p.format); err != nil {
		return err
	}

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

func parseRenderRangeParams(r *http.Request, from, until string) (startTime, endTime time.Time, ok bool) {

	switch r.Method {
	case http.MethodGet:
		fromStr, untilStr := r.URL.Query().Get(from), r.URL.Query().Get(until)
		startTime, endTime = attime.Parse(fromStr), attime.Parse(untilStr)
		return startTime, endTime, fromStr != "" || untilStr != ""
	case http.MethodPost:
		startTime, endTime = attime.Parse(from), attime.Parse(until)
		return startTime, endTime, from != "" || until != ""
	}

	return time.Now(), time.Now(), false

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

// Request Body Interface
type RenderDiffParams struct {
	Name  *string `json:"name,omitempty"`
	Query *string `json:"query,omitempty"`

	From  string `json:"from"`
	Until string `json:"until"`

	Format   string `json:"format"`
	MaxNodes *int   `json:"maxNodes,omitempty"`

	Left  RenderTreeParams `json:"leftParams"`
	Right RenderTreeParams `json:"rightParams"`
}

type RenderTreeParams struct {
	From  string `json:"from"`
	Until string `json:"until"`
}
