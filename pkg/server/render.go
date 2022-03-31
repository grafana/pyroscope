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

type RenderTreeParams struct {
	From  string `json:"from"`
	Until string `json:"until"`
}
