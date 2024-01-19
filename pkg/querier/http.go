package querier

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	"github.com/gogo/status"
	"github.com/google/pprof/profile"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/frontend/dot/graph"
	"github.com/grafana/pyroscope/pkg/frontend/dot/report"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/og/structs/flamebearer"
	"github.com/grafana/pyroscope/pkg/og/util/attime"
	"github.com/grafana/pyroscope/pkg/querier/timeline"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
)

func NewHTTPHandlers(client querierv1connect.QuerierServiceClient) *QueryHandlers {
	return &QueryHandlers{client}
}

type QueryHandlers struct {
	client querierv1connect.QuerierServiceClient
}

// LabelValues only returns the label values for the given label name.
// This is mostly for fulfilling the pyroscope API and won't be used in the future.
// For example, /label-values?label=__name__ will return all the profile types.
func (q *QueryHandlers) LabelValues(w http.ResponseWriter, req *http.Request) {
	label := req.URL.Query().Get("label")
	if label == "" {
		httputil.Error(w, connect.NewError(connect.CodeInvalidArgument, errors.New("label parameter is required")))
		return
	}
	var res []string

	if label == "__name__" {
		response, err := q.client.ProfileTypes(req.Context(), connect.NewRequest(&querierv1.ProfileTypesRequest{}))
		if err != nil {
			httputil.Error(w, err)
			return
		}
		for _, t := range response.Msg.ProfileTypes {
			res = append(res, t.ID)
		}
	} else {
		response, err := q.client.LabelValues(req.Context(), connect.NewRequest(&typesv1.LabelValuesRequest{}))
		if err != nil {
			httputil.Error(w, err)
			return
		}
		res = response.Msg.Names
	}

	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(res); err != nil {
		httputil.Error(w, err)
		return
	}
}

func (q *QueryHandlers) RenderDiff(w http.ResponseWriter, req *http.Request) {
	if err := req.ParseForm(); err != nil {
		httputil.Error(w, connect.NewError(connect.CodeInvalidArgument, err))
		return
	}

	// Left
	leftSelectParams, leftProfileType, err := parseSelectProfilesRequest(renderRequestFieldNames{
		query: "leftQuery",
		from:  "leftFrom",
		until: "leftUntil",
	}, req)
	if err != nil {
		httputil.Error(w, connect.NewError(connect.CodeInvalidArgument, err))
		return
	}

	rightSelectParams, rightProfileType, err := parseSelectProfilesRequest(renderRequestFieldNames{
		query: "rightQuery",
		from:  "rightFrom",
		until: "rightUntil",
	}, req)
	if err != nil {
		httputil.Error(w, connect.NewError(connect.CodeInvalidArgument, err))
		return
	}
	// TODO: check profile types?
	if leftProfileType.ID != rightProfileType.ID {
		httputil.Error(w, connect.NewError(connect.CodeInvalidArgument, errors.New("profile types must match")))
		return
	}

	res, err := q.client.Diff(req.Context(), connect.NewRequest(&querierv1.DiffRequest{
		Left:  leftSelectParams,
		Right: rightSelectParams,
	}))
	if err != nil {
		httputil.Error(w, err)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(phlaremodel.ExportDiffToFlamebearer(res.Msg.Flamegraph, leftProfileType)); err != nil {
		httputil.Error(w, err)
		return
	}
}

func (q *QueryHandlers) Render(w http.ResponseWriter, req *http.Request) {
	if err := req.ParseForm(); err != nil {
		httputil.Error(w, connect.NewError(connect.CodeInvalidArgument, err))
		return
	}
	selectParams, profileType, err := parseSelectProfilesRequest(renderRequestFieldNames{}, req)
	if err != nil {
		httputil.Error(w, connect.NewError(connect.CodeInvalidArgument, err))
		return
	}

	groupBy := req.URL.Query()["groupBy"]
	var aggregation typesv1.TimeSeriesAggregationType
	if req.URL.Query().Has("aggregation") {
		aggregationParam := req.URL.Query().Get("aggregation")
		switch aggregationParam {
		case "sum":
			aggregation = typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_SUM
		case "avg":
			aggregation = typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_AVERAGE
		}
	}

	format := req.URL.Query().Get("format")
	if format == "dot" {
		// We probably should distinguish max nodes of the source pprof
		// profile and max nodes value for the output profile in dot format.
		sourceProfileMaxNodes := int64(512)
		dotProfileMaxNodes := int64(100)
		if selectParams.MaxNodes != nil {
			if v := *selectParams.MaxNodes; v > 0 {
				dotProfileMaxNodes = v
			}
			if dotProfileMaxNodes > sourceProfileMaxNodes {
				sourceProfileMaxNodes = dotProfileMaxNodes
			}
		}
		resp, err := q.client.SelectMergeProfile(req.Context(), connect.NewRequest(&querierv1.SelectMergeProfileRequest{
			Start:         selectParams.Start,
			End:           selectParams.End,
			ProfileTypeID: selectParams.ProfileTypeID,
			LabelSelector: selectParams.LabelSelector,
			MaxNodes:      &sourceProfileMaxNodes,
		}))
		if err != nil {
			httputil.Error(w, connect.NewError(connect.CodeInternal, err))
			return
		}
		if err = pprofToDotProfile(w, resp.Msg, int(dotProfileMaxNodes)); err != nil {
			httputil.Error(w, connect.NewError(connect.CodeInternal, err))
		}
		return
	}

	var resFlame *connect.Response[querierv1.SelectMergeStacktracesResponse]
	g, ctx := errgroup.WithContext(req.Context())
	selectParamsClone := selectParams.CloneVT()
	g.Go(func() error {
		var err error
		resFlame, err = q.client.SelectMergeStacktraces(ctx, connect.NewRequest(selectParamsClone))
		return err
	})

	timelineStep := timeline.CalcPointInterval(selectParams.Start, selectParams.End)
	var resSeries *connect.Response[querierv1.SelectSeriesResponse]
	g.Go(func() error {
		var err error
		resSeries, err = q.client.SelectSeries(req.Context(),
			connect.NewRequest(&querierv1.SelectSeriesRequest{
				ProfileTypeID: selectParams.ProfileTypeID,
				LabelSelector: selectParams.LabelSelector,
				Start:         selectParams.Start,
				End:           selectParams.End,
				Step:          timelineStep,
				GroupBy:       groupBy,
				Aggregation:   &aggregation,
			}))

		return err
	})

	err = g.Wait()
	if err != nil {
		httputil.Error(w, err)
		return
	}

	seriesVal := &typesv1.Series{}
	if len(resSeries.Msg.Series) == 1 {
		seriesVal = resSeries.Msg.Series[0]
	}

	fb := phlaremodel.ExportToFlamebearer(resFlame.Msg.Flamegraph, profileType)
	fb.Timeline = timeline.New(seriesVal, selectParams.Start, selectParams.End, int64(timelineStep))

	if len(groupBy) > 0 {
		fb.Groups = make(map[string]*flamebearer.FlamebearerTimelineV1)
		for _, s := range resSeries.Msg.Series {
			key := "*"
			for _, l := range s.Labels {
				// right now we only support one group by
				if l.Name == groupBy[0] {
					key = l.Value
					break
				}
			}
			fb.Groups[key] = timeline.New(s, selectParams.Start, selectParams.End, int64(timelineStep))
		}
	}

	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(fb); err != nil {
		httputil.Error(w, err)
		return
	}
}

func pprofToDotProfile(w io.Writer, p *profilev1.Profile, maxNodes int) error {
	data, err := p.MarshalVT()
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	pr, err := profile.ParseData(data)
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	rpt := report.NewDefault(pr, report.Options{NodeCount: maxNodes})
	gr, cfg := report.GetDOT(rpt)
	graph.ComposeDot(w, gr, &graph.DotAttributes{}, cfg)
	return nil
}

type renderRequestFieldNames struct {
	query string
	from  string
	until string
}

// render/render?format=json&from=now-12h&until=now&query=pyroscope.server.cpu
func parseSelectProfilesRequest(fieldNames renderRequestFieldNames, req *http.Request) (*querierv1.SelectMergeStacktracesRequest, *typesv1.ProfileType, error) {
	if fieldNames == (renderRequestFieldNames{}) {
		fieldNames = renderRequestFieldNames{
			query: "query",
			from:  "from",
			until: "until",
		}
	}
	selector, ptype, err := parseQuery(fieldNames.query, req)
	if err != nil {
		return nil, nil, err
	}

	v := req.URL.Query()

	// parse time using pyroscope's attime parser
	start := model.TimeFromUnixNano(attime.Parse(v.Get(fieldNames.from)).UnixNano())
	end := model.TimeFromUnixNano(attime.Parse(v.Get(fieldNames.until)).UnixNano())

	p := &querierv1.SelectMergeStacktracesRequest{
		Start:         int64(start),
		End:           int64(end),
		LabelSelector: selector,
		ProfileTypeID: ptype.ID,
	}

	var mn int64
	if v, err := strconv.Atoi(v.Get("max-nodes")); err == nil && v != 0 {
		mn = int64(v)
	}
	if v, err := strconv.Atoi(v.Get("maxNodes")); err == nil && v != 0 {
		mn = int64(v)
	}
	p.MaxNodes = &mn

	return p, ptype, nil
}

func parseQuery(fieldName string, req *http.Request) (string, *typesv1.ProfileType, error) {
	q := req.Form.Get(fieldName)
	if q == "" {
		return "", nil, fmt.Errorf("'%s' is required", fieldName)
	}

	parsedSelector, err := parser.ParseMetricSelector(q)
	if err != nil {
		return "", nil, status.Error(codes.InvalidArgument, fmt.Sprintf("failed to parse '%s'", fieldName))
	}

	sel := make([]*labels.Matcher, 0, len(parsedSelector))
	var nameLabel *labels.Matcher
	for _, matcher := range parsedSelector {
		if matcher.Name == labels.MetricName {
			nameLabel = matcher
		} else {
			sel = append(sel, matcher)
		}
	}
	if nameLabel == nil {
		return "", nil, status.Error(codes.InvalidArgument, fmt.Sprintf("'%s' must contain a profile-type selection", fieldName))
	}

	profileSelector, err := phlaremodel.ParseProfileTypeSelector(nameLabel.Value)
	if err != nil {
		return "", nil, status.Error(codes.InvalidArgument, fmt.Sprintf("failed to parse '%s'", fieldName))
	}
	return convertMatchersToString(sel), profileSelector, nil
}

func convertMatchersToString(matchers []*labels.Matcher) string {
	out := strings.Builder{}
	out.WriteRune('{')

	for idx, m := range matchers {
		if idx > 0 {
			out.WriteRune(',')
		}

		out.WriteString(m.String())
	}

	out.WriteRune('}')
	return out.String()
}
