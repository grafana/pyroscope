package querier

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/gogo/status"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"google.golang.org/grpc/codes"

	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	querierv1 "github.com/grafana/fire/pkg/gen/querier/v1"
	firemodel "github.com/grafana/fire/pkg/model"
)

// LabelValuesHandler only returns the label values for the given label name.
// This is mostly for fulfilling the pyroscope API and won't be used in the future.
// /label-values?label=__name__
func (q *Querier) LabelValuesHandler(w http.ResponseWriter, req *http.Request) {
	label := req.URL.Query().Get("label")
	if label == "" {
		http.Error(w, "label parameter is required", http.StatusBadRequest)
		return
	}
	var (
		res []string
		err error
	)

	if label == "__name__" {
		response, err := q.ProfileTypes(req.Context(), connect.NewRequest(&querierv1.ProfileTypesRequest{}))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, t := range response.Msg.ProfileTypes {
			res = append(res, t.ID)
		}
	} else {
		response, err := q.LabelValues(req.Context(), connect.NewRequest(&querierv1.LabelValuesRequest{}))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		res = response.Msg.Names
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(res); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (q *Querier) RenderHandler(w http.ResponseWriter, req *http.Request) {
	if err := req.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	selectParams, profileType, err := parseSelectProfilesRequest(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := q.SelectMergeStacktraces(req.Context(), connect.NewRequest(selectParams))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(ExportToFlamebearer(res.Msg.Flamegraph, profileType)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// render/render?format=json&from=now-12h&until=now&query=pyroscope.server.cpu
func parseSelectProfilesRequest(req *http.Request) (*querierv1.SelectMergeStacktracesRequest, *commonv1.ProfileType, error) {
	selector, ptype, err := parseQuery(req)
	if err != nil {
		return nil, nil, err
	}

	// default start and end to now-1h
	start := model.TimeFromUnixNano(time.Now().Add(-1 * time.Hour).UnixNano())
	end := model.TimeFromUnixNano(time.Now().UnixNano())

	if from := req.Form.Get("from"); from != "" {
		from, err := parseRelativeTime(from)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse from: %w", err)
		}
		start = end.Add(-from)
	}
	return &querierv1.SelectMergeStacktracesRequest{
		Start:         int64(start),
		End:           int64(end),
		LabelSelector: selector,
		ProfileTypeID: ptype.ID,
	}, ptype, nil
}

func parseQuery(req *http.Request) (string, *commonv1.ProfileType, error) {
	q := req.Form.Get("query")
	if q == "" {
		return "", nil, fmt.Errorf("query is required")
	}

	parsedSelector, err := parser.ParseMetricSelector(q)
	if err != nil {
		return "", nil, status.Error(codes.InvalidArgument, "failed to parse query")
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
		return "", nil, status.Error(codes.InvalidArgument, "query must contain a profile-type selection")
	}

	profileSelector, err := firemodel.ParseProfileTypeSelector(nameLabel.Value)
	if err != nil {
		return "", nil, status.Error(codes.InvalidArgument, "failed to parse query")
	}
	return convertMatchersToString(sel), profileSelector, nil
}

func parseRelativeTime(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "now-")

	d, err := model.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	return time.Duration(d), nil
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
