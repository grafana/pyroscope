package querier

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gogo/status"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"google.golang.org/grpc/codes"

	ingesterv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
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
		res, err = q.ProfileTypes(req.Context())
	} else {
		res, err = q.LabelValues(req.Context(), label)
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
	selectParams, err := parseSelectProfilesRequest(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	flame, err := q.selectMerge(req.Context(), selectParams)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(flame); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// render/render?format=json&from=now-12h&until=now&query=pyroscope.server.cpu
func parseSelectProfilesRequest(req *http.Request) (*ingesterv1.SelectProfilesRequest, error) {
	queryParams := req.URL.Query()
	q := queryParams.Get("query")
	if q == "" {
		return nil, fmt.Errorf("query is required")
	}

	parsedSelector, err := parser.ParseMetricSelector(q)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "failed to parse query")
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
		return nil, status.Error(codes.InvalidArgument, "query must contain a profile-type selection")
	}

	parts := strings.Split(nameLabel.Value, ":")
	if len(parts) != 5 && len(parts) != 6 {
		return nil, status.Errorf(codes.InvalidArgument, "profile-type selection must be of the form <name>:<sample-type>:<sample-unit>:<period-type>:<period-unit>(:delta), got(%d): %q", len(parts), nameLabel.Value)
	}
	name, sampleType, sampleUnit, periodType, periodUnit := parts[0], parts[1], parts[2], parts[3], parts[4]

	// default start and end to now-1h
	start := model.TimeFromUnixNano(time.Now().Add(-1 * time.Hour).UnixNano())
	end := model.TimeFromUnixNano(time.Now().UnixNano())

	if from := queryParams.Get("from"); from != "" {
		from, err := parseRelativeTime(from)
		if err != nil {
			return nil, fmt.Errorf("failed to parse from: %w", err)
		}
		start = end.Add(-from)
	}
	return &ingesterv1.SelectProfilesRequest{
		Start:         int64(start),
		End:           int64(end),
		LabelSelector: convertMatchersToString(sel),
		Type: &ingesterv1.ProfileType{
			Name:       name,
			SampleType: sampleType,
			SampleUnit: sampleUnit,
			PeriodType: periodType,
			PeriodUnit: periodUnit,
		},
	}, nil
}

func parseRelativeTime(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "now-")
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	return d, nil
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
