package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"

	"github.com/pyroscope-io/pyroscope/pkg/api"
	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/history"
	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
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

type annotationsResponse struct {
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}
type renderResponse struct {
	flamebearer.FlamebearerProfile
	Metadata    renderMetadataResponse `json:"metadata"`
	Annotations []annotationsResponse  `json:"annotations"`
}

type RenderHandler struct {
	log                *logrus.Logger
	storage            storage.Getter
	dir                http.FileSystem
	stats              StatsReceiver
	maxNodesDefault    int
	httpUtils          httputils.Utils
	historyMgr         history.Manager
	annotationsService api.AnnotationsService
}

func (ctrl *Controller) renderHandler() http.HandlerFunc {
	return NewRenderHandler(ctrl.log, ctrl.storage, ctrl.dir, ctrl, ctrl.config.MaxNodesRender, ctrl.httpUtils, ctrl.historyMgr, ctrl.annotationsService).ServeHTTP
}

//revive:disable:argument-limit TODO(petethepig): we will refactor this later
func NewRenderHandler(
	l *logrus.Logger,
	s storage.Getter,
	dir http.FileSystem,
	stats StatsReceiver,
	maxNodesDefault int,
	httpUtils httputils.Utils,
	historyMgr history.Manager,
	annotationsService api.AnnotationsService,
) *RenderHandler {
	return &RenderHandler{
		log:                l,
		storage:            s,
		dir:                dir,
		stats:              stats,
		maxNodesDefault:    maxNodesDefault,
		httpUtils:          httpUtils,
		historyMgr:         historyMgr,
		annotationsService: annotationsService,
	}
}

func (rh *RenderHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var p renderParams
	if err := rh.renderParametersFromRequest(r, &p); err != nil {
		rh.httpUtils.WriteInvalidParameterError(r, w, err)
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
		rh.httpUtils.WriteInternalServerError(r, w, err, "failed to retrieve data")
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
		flame := flamebearer.NewProfile(flamebearer.ProfileConfig{
			Name:      filename,
			MaxNodes:  p.maxNodes,
			Tree:      out.Tree,
			Timeline:  out.Timeline,
			Groups:    out.Groups,
			Telemetry: out.Telemetry,
			Metadata: metadata.Metadata{
				SpyName:         out.SpyName,
				SampleRate:      out.SampleRate,
				Units:           out.Units,
				AggregationType: out.AggregationType,
			},
		})

		// Look up annotations
		annotations, err := rh.annotationsService.FindAnnotationsByTimeRange(r.Context(), appName, p.gi.StartTime, p.gi.EndTime)
		if err != nil {
			rh.log.Error(err)
			// it's better to not show any annotations than falling the entire request
			annotations = []model.Annotation{}
		}

		res := rh.mountRenderResponse(flame, appName, p.gi, p.maxNodes, annotations)
		rh.httpUtils.WriteResponseJSON(r, w, res)
	case "pprof":
		pprof := out.Tree.Pprof(&tree.PprofMetadata{
			// TODO(petethepig): not sure if this conversion is right
			Unit:      string(out.Units),
			StartTime: p.gi.StartTime,
		})
		out, err := proto.Marshal(pprof)
		if err == nil {
			rh.httpUtils.WriteResponseFile(r, w, fmt.Sprintf("%v.pprof", filename), out)
		} else {
			rh.httpUtils.WriteInternalServerError(r, w, err, "failed to serialize data")
		}
	case "collapsed":
		collapsed := out.Tree.Collapsed()
		rh.httpUtils.WriteResponseFile(r, w, fmt.Sprintf("%v.collapsed.txt", filename), []byte(collapsed))
	case "html":
		res := flamebearer.NewProfile(flamebearer.ProfileConfig{
			Name:      filename,
			MaxNodes:  p.maxNodes,
			Tree:      out.Tree,
			Timeline:  out.Timeline,
			Groups:    out.Groups,
			Telemetry: out.Telemetry,
			Metadata: metadata.Metadata{
				SpyName:         out.SpyName,
				SampleRate:      out.SampleRate,
				Units:           out.Units,
				AggregationType: out.AggregationType,
			},
		})
		w.Header().Add("Content-Type", "text/html")
		if err := flamebearer.FlamebearerToStandaloneHTML(&res, rh.dir, w); err != nil {
			rh.httpUtils.WriteJSONEncodeError(r, w, err)
			return
		}
	}
}

// Enhance the flamebearer with a few additional fields the UI requires
func (*RenderHandler) mountRenderResponse(flame flamebearer.FlamebearerProfile, appName string, gi *storage.GetInput, maxNodes int, annotations []model.Annotation) renderResponse {
	md := renderMetadataResponse{
		FlamebearerMetadataV1: flame.Metadata,
		AppName:               appName,
		StartTime:             gi.StartTime.Unix(),
		EndTime:               gi.EndTime.Unix(),
		Query:                 gi.Query.String(),
		MaxNodes:              maxNodes,
	}

	annotationsResp := make([]annotationsResponse, len(annotations))
	for i, an := range annotations {
		annotationsResp[i] = annotationsResponse{
			Content:   an.Content,
			Timestamp: an.Timestamp.Unix(),
		}
	}

	return renderResponse{
		FlamebearerProfile: flame,
		Metadata:           md,
		Annotations:        annotationsResp,
	}
}

func (rh *RenderHandler) renderParametersFromRequest(r *http.Request, p *renderParams) error {
	v := r.URL.Query()
	p.gi = new(storage.GetInput)

	k := v.Get("name")
	q := v.Get("query")
	p.gi.GroupBy = v.Get("groupBy")

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
	if newMaxNodes, ok := MaxNodesFromContext(r.Context()); ok {
		p.maxNodes = newMaxNodes
	}
	if mn, err := strconv.Atoi(v.Get("max-nodes")); err == nil && mn != 0 {
		p.maxNodes = mn
	}
	if mn, err := strconv.Atoi(v.Get("maxNodes")); err == nil && mn != 0 {
		p.maxNodes = mn
	}

	p.gi.StartTime = attime.Parse(v.Get("from"))
	p.gi.EndTime = attime.Parse(v.Get("until"))
	p.format = v.Get("format")

	return expectFormats(p.format)
}
