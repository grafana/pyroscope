package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"

	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
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

	leftStartTime time.Time
	leftEndTime   time.Time
	rghtStartTime time.Time
	rghtEndTime   time.Time
}

type renderMetadataResponse struct {
	flamebearer.FlamebearerMetadataV1
	AppName   string `json:"appName"`
	StartTime int64  `json:"startTime"`
	EndTime   int64  `json:"endTime"`
	Query     string `json:"query"`
	MaxNodes  int    `json:"maxNodes"`
}
type RenderResponse struct {
	flamebearer.FlamebearerProfile
	Metadata renderMetadataResponse `json:"metadata"`
}

func (ctrl *Controller) renderHandler(w http.ResponseWriter, r *http.Request) {
	var p renderParams
	if err := ctrl.renderParametersFromRequest(r, &p); err != nil {
		ctrl.writeInvalidParameterError(w, err)
		return
	}

	if err := ctrl.expectFormats(p.format); err != nil {
		ctrl.writeInvalidParameterError(w, errUnknownFormat)
		return
	}

	out, err := ctrl.storage.Get(p.gi)
	var appName string
	if p.gi.Key != nil {
		appName = p.gi.Key.AppName()
	} else if p.gi.Query != nil {
		appName = p.gi.Query.AppName
	}
	filename := fmt.Sprintf("%v %v", appName, p.gi.StartTime.UTC().Format(time.RFC3339))
	ctrl.statsInc("render")
	if err != nil {
		ctrl.writeInternalServerError(w, err, "failed to retrieve data")
		return
	}
	// TODO: handle properly
	if out == nil {
		out = &storage.GetOutput{Tree: tree.New()}
	}

	switch p.format {
	case "json":
		flame := flamebearer.NewProfile(filename, out, p.maxNodes)
		res := ctrl.mountRenderResponse(flame, appName, p.gi, p.maxNodes)
		ctrl.writeResponseJSON(w, res)
	case "pprof":
		pprof := out.Tree.Pprof(&tree.PprofMetadata{
			Unit:      out.Units,
			StartTime: p.gi.StartTime,
		})
		out, err := proto.Marshal(pprof)
		if err == nil {
			ctrl.writeResponseFile(w, fmt.Sprintf("%v.pprof", filename), out)
		} else {
			ctrl.writeInternalServerError(w, err, "failed to serialize data")
		}
	case "collapsed":
		collapsed := out.Tree.Collapsed()
		ctrl.writeResponseFile(w, fmt.Sprintf("%v.collapsed.txt", filename), []byte(collapsed))
	case "html":
		res := flamebearer.NewProfile(filename, out, p.maxNodes)
		w.Header().Add("Content-Type", "text/html")
		if err := flamebearer.FlamebearerToStandaloneHTML(&res, ctrl.dir, w); err != nil {
			ctrl.writeJSONEncodeError(w, err)
			return
		}
	}
}

type mergeRequest struct {
	AppName  string   `json:"appName"`
	Profiles []string `json:"profiles"`
	MaxNodes int      `json:"maxNodes"`
}

type mergeResponse struct {
	flamebearer.FlamebearerProfile
}

func (ctrl *Controller) mergeHandler(w http.ResponseWriter, r *http.Request) {
	var req mergeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ctrl.writeInvalidParameterError(w, err)
		return
	}

	if req.AppName == "" {
		ctrl.writeInvalidParameterError(w, fmt.Errorf("application name required"))
		return
	}
	if len(req.Profiles) == 0 {
		ctrl.writeInvalidParameterError(w, fmt.Errorf("at least one profile ID must be specified"))
		return
	}
	maxNodes := ctrl.config.MaxNodesRender
	if req.MaxNodes > 0 {
		maxNodes = req.MaxNodes
	}

	out, err := ctrl.storage.MergeProfiles(r.Context(), storage.MergeProfilesInput{
		AppName:  req.AppName,
		Profiles: req.Profiles,
	})
	if err != nil {
		ctrl.writeInternalServerError(w, err, "failed to retrieve data")
		return
	}

	flame := out.Tree.FlamebearerStruct(maxNodes)
	resp := mergeResponse{
		FlamebearerProfile: flamebearer.FlamebearerProfile{
			Version: 1,
			FlamebearerProfileV1: flamebearer.FlamebearerProfileV1{
				Flamebearer: flamebearer.FlamebearerV1{
					Names:    flame.Names,
					Levels:   flame.Levels,
					NumTicks: flame.NumTicks,
					MaxSelf:  flame.MaxSelf,
				},
				// Hardcoded values for Go.
				Metadata: flamebearer.FlamebearerMetadataV1{
					Format:     string(tree.FormatSingle),
					SpyName:    "unknown",
					SampleRate: 100,
					Units:      "samples",
				},
			},
		},
	}

	ctrl.statsInc("merge")
	ctrl.writeResponseJSON(w, resp)
}

// Enhance the flamebearer with a few additional fields the UI requires
func (*Controller) mountRenderResponse(flame flamebearer.FlamebearerProfile, appName string, gi *storage.GetInput, maxNodes int) RenderResponse {
	metadata := renderMetadataResponse{
		flame.Metadata,
		appName,
		gi.StartTime.Unix(),
		gi.EndTime.Unix(),
		gi.Query.String(),
		maxNodes,
	}

	renderResponse := RenderResponse{
		flame,
		metadata,
	}

	return renderResponse
}

// parseQueryParams parses query params into a GetInput
func (ctrl *Controller) parseQueryParams(r *http.Request, prefix string) (gi storage.GetInput, err error) {
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

type DiffParams struct {
	Left  storage.GetInput
	Right storage.GetInput

	Format   string
	MaxNodes int
}

func (ctrl *Controller) parseDiffParams2(r *http.Request, p *DiffParams) (err error) {
	p.Left, err = ctrl.parseQueryParams(r, "left")
	if err != nil {
		return fmt.Errorf("%q: %+w", "Could not parse 'left' side", err)
	}

	p.Right, err = ctrl.parseQueryParams(r, "right")
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

func (ctrl *Controller) renderDiffHandler2(w http.ResponseWriter, r *http.Request) {
	var (
		params DiffParams
	)

	switch r.Method {
	case http.MethodGet:
		if err := ctrl.parseDiffParams2(r, &params); err != nil {
			ctrl.writeInvalidParameterError(w, err)
			return
		}
	default:
		ctrl.writeInvalidMethodError(w)
		return
	}

	// TODO: do this concurrently
	// Left Tree
	// TODO: why do we need to pass this?
	leftOut, leftErr := ctrl.loadTree(&params.Left, params.Left.StartTime, params.Left.EndTime)
	if leftErr != nil {
		panic("TODO")
	}
	rightOut, rightErr := ctrl.loadTree(&params.Right, params.Right.StartTime, params.Right.EndTime)
	if rightErr != nil {
		panic("TODO")
	}

	// It seems Out is used for the timeline
	// Which we don't care for Diff, since the timeline in the frontend
	// Is generated by plotting left AND right
	var noopOut storage.GetOutput
	combined := flamebearer.NewCombinedProfile("diff", &noopOut, leftOut, rightOut, params.MaxNodes)

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
		// TODO: mount a gi
		//		res := ctrl.mountRenderResponse(combined, "diff", leftOut, p.MaxNodes)
		ctrl.writeResponseJSON(w, combined)
	}
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
		ctrl.writeInvalidMethodError(w)
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

	var appName string
	if p.gi.Key != nil {
		appName = p.gi.Key.AppName()
	} else if p.gi.Query != nil {
		appName = p.gi.Query.AppName
	}
	combined := flamebearer.NewCombinedProfile(appName, out, leftOut, rghtOut, p.maxNodes)

	switch p.format {
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
		res := ctrl.mountRenderResponse(combined, appName, p.gi, p.maxNodes)
		ctrl.writeResponseJSON(w, res)
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

	return ctrl.expectFormats(p.format)
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

	return ctrl.expectFormats(p.format)
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

//revive:disable-next-line:argument-limit 7 parameters here is fine
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

type RenderDiffParams2 struct {
	LeftQuery string `json:"leftQuery"`
	LeftFrom  string `json:"leftFrom"`
	LeftUntil string `json:"leftUntil"`

	RightQuery string `json:"rightQuery"`
	RightFrom  string `json:"rightFrom"`
	RightUntil string `json:"rightUntil"`

	Format   string `json:"format"`
	MaxNodes *int   `json:"maxNodes,omitempty"`
}

type RenderTreeParams struct {
	From  string `json:"from"`
	Until string `json:"until"`
}
