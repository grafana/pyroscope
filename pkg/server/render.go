package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"

	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
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

type RenderHandler struct {
	log             *logrus.Logger
	storage         storage.Getter
	dir             http.FileSystem
	stats           StatsReceiver
	maxNodesDefault int
	httpUtils       httputils.Utils
}

func (ctrl *Controller) renderHandler() http.HandlerFunc {
	return NewRenderHandler(ctrl.log, ctrl.storage, ctrl.dir, ctrl, ctrl.config.MaxNodesRender, ctrl.httpUtils).ServeHTTP
}

//revive:disable:argument-limit TODO(petethepig): we will refactor this later
func NewRenderHandler(l *logrus.Logger, s storage.Getter, dir http.FileSystem, stats StatsReceiver, maxNodesDefault int, httpUtils httputils.Utils) *RenderHandler {
	return &RenderHandler{
		log:             l,
		storage:         s,
		dir:             dir,
		stats:           stats,
		maxNodesDefault: maxNodesDefault,
		httpUtils:       httpUtils,
	}
}

func (rh *RenderHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var p renderParams
	if err := rh.renderParametersFromRequest(r, &p); err != nil {
		rh.httpUtils.WriteInvalidParameterError(w, err)
		return
	}

	if err := expectFormats(p.format); err != nil {
		rh.httpUtils.WriteInvalidParameterError(w, errUnknownFormat)
		return
	}

	out, err := rh.storage.Get(r.Context(), p.gi)
	var appName string
	if p.gi.Key != nil {
		appName = p.gi.Key.AppName()
	} else if p.gi.Query != nil {
		appName = p.gi.Query.AppName
	}
	filename := fmt.Sprintf("%v %v", appName, p.gi.StartTime.UTC().Format(time.RFC3339))
	rh.stats.StatsInc("render")
	if err != nil {
		rh.httpUtils.WriteInternalServerError(w, err, "failed to retrieve data")
		return
	}
	if out == nil {
		out = &storage.GetOutput{
			Tree:     tree.New(),
			Timeline: segment.GenerateTimeline(p.gi.StartTime, p.gi.EndTime),
		}
	}

	switch p.format {
	case "json":
		flame := flamebearer.NewProfile(filename, out, p.maxNodes)
		res := rh.mountRenderResponse(flame, appName, p.gi, p.maxNodes)
		rh.httpUtils.WriteResponseJSON(w, res)
	case "pprof":
		pprof := out.Tree.Pprof(&tree.PprofMetadata{
			Unit:      out.Units,
			StartTime: p.gi.StartTime,
		})
		out, err := proto.Marshal(pprof)
		if err == nil {
			rh.httpUtils.WriteResponseFile(w, fmt.Sprintf("%v.pprof", filename), out)
		} else {
			rh.httpUtils.WriteInternalServerError(w, err, "failed to serialize data")
		}
	case "collapsed":
		collapsed := out.Tree.Collapsed()
		rh.httpUtils.WriteResponseFile(w, fmt.Sprintf("%v.collapsed.txt", filename), []byte(collapsed))
	case "html":
		res := flamebearer.NewProfile(filename, out, p.maxNodes)
		w.Header().Add("Content-Type", "text/html")
		if err := flamebearer.FlamebearerToStandaloneHTML(&res, rh.dir, w); err != nil {
			rh.httpUtils.WriteJSONEncodeError(w, err)
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

// Enhance the flamebearer with a few additional fields the UI requires
func (*RenderHandler) mountRenderResponse(flame flamebearer.FlamebearerProfile, appName string, gi *storage.GetInput, maxNodes int) RenderResponse {
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

func (rh *RenderHandler) renderParametersFromRequest(r *http.Request, p *renderParams) error {
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

	p.maxNodes = rh.maxNodesDefault
	if mn, err := strconv.Atoi(v.Get("max-nodes")); err == nil && mn > 0 {
		p.maxNodes = mn
	}

	p.gi.StartTime = attime.Parse(v.Get("from"))
	p.gi.EndTime = attime.Parse(v.Get("until"))
	p.format = v.Get("format")

	return expectFormats(p.format)
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

type RenderTreeParams struct {
	From  string `json:"from"`
	Until string `json:"until"`
}
