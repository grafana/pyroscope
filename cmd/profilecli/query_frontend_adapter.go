package main

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/query/v1/queryv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/pprof"
)

// queryProfileViaFrontend queries a merged pprof profile via QueryFrontendService.
func queryProfileViaFrontend(ctx context.Context, client queryv1connect.QueryFrontendServiceClient, req *querierv1.SelectMergeProfileRequest, async bool) (*googlev1.Profile, error) {
	qr := &queryv1.QueryRequest{
		StartTime:     req.Start,
		EndTime:       req.End,
		LabelSelector: labelSelectorWithProfileType(req.LabelSelector, req.ProfileTypeID),
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_PPROF,
			Pprof: &queryv1.PprofQuery{
				MaxNodes:           req.GetMaxNodes(),
				StackTraceSelector: req.StackTraceSelector,
				ProfileIdSelector:  req.ProfileIdSelector,
			},
		}},
	}

	resp, err := queryViaFrontendService(ctx, client, qr, async)
	if err != nil {
		return nil, err
	}

	return extractProfileFromResponse(resp)
}

// querySeriesViaFrontend queries series labels via QueryFrontendService.
func querySeriesViaFrontend(ctx context.Context, client queryv1connect.QueryFrontendServiceClient, req *querierv1.SeriesRequest, async bool) ([]*typesv1.Labels, error) {
	qr := &queryv1.QueryRequest{
		StartTime:     req.Start,
		EndTime:       req.End,
		LabelSelector: matchersToLabelSelector(req.Matchers),
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_SERIES_LABELS,
			SeriesLabels: &queryv1.SeriesLabelsQuery{
				LabelNames: req.LabelNames,
			},
		}},
	}

	resp, err := queryViaFrontendService(ctx, client, qr, async)
	if err != nil {
		return nil, err
	}

	if len(resp.Reports) == 0 {
		return nil, nil
	}
	report := resp.Reports[0]
	if report.SeriesLabels == nil {
		return nil, fmt.Errorf("unexpected report type: expected series_labels")
	}
	return report.SeriesLabels.SeriesLabels, nil
}

// querySelectSeriesViaFrontend queries time series via QueryFrontendService.
func querySelectSeriesViaFrontend(ctx context.Context, client queryv1connect.QueryFrontendServiceClient, req *querierv1.SelectSeriesRequest, async bool) ([]*typesv1.Series, error) {
	qr := &queryv1.QueryRequest{
		StartTime:     req.Start,
		EndTime:       req.End,
		LabelSelector: labelSelectorWithProfileType(req.LabelSelector, req.ProfileTypeID),
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TIME_SERIES,
			TimeSeries: &queryv1.TimeSeriesQuery{
				Step:         req.Step,
				GroupBy:      req.GroupBy,
				ExemplarType: req.ExemplarType,
			},
		}},
	}

	resp, err := queryViaFrontendService(ctx, client, qr, async)
	if err != nil {
		return nil, err
	}

	if len(resp.Reports) == 0 {
		return nil, nil
	}
	report := resp.Reports[0]
	if report.TimeSeries == nil {
		return nil, fmt.Errorf("unexpected report type: expected time_series")
	}
	return report.TimeSeries.TimeSeries, nil
}

// labelSelectorWithProfileType combines a label selector with a profile type ID.
func labelSelectorWithProfileType(labelSelector, profileTypeID string) string {
	profileType, err := model.ParseProfileTypeSelector(profileTypeID)
	if err != nil {
		return labelSelector
	}
	ptMatcher := model.SelectorFromProfileType(profileType)
	if labelSelector == "" || labelSelector == "{}" {
		return "{" + ptMatcher.String() + "}"
	}
	return labelSelector[:len(labelSelector)-1] + "," + ptMatcher.String() + "}"
}

// matchersToLabelSelector converts a list of matchers strings to a single label selector.
func matchersToLabelSelector(matchers []string) string {
	if len(matchers) == 0 {
		return "{}"
	}
	return matchers[0]
}

// extractProfileFromResponse extracts a pprof profile from a QueryResponse.
func extractProfileFromResponse(resp *queryv1.QueryResponse) (*googlev1.Profile, error) {
	if len(resp.Reports) == 0 {
		return &googlev1.Profile{}, nil
	}
	report := resp.Reports[0]
	if report.Pprof == nil {
		return nil, errors.New("unexpected report type: expected pprof")
	}
	var p googlev1.Profile
	if err := pprof.Unmarshal(report.Pprof.Pprof, &p); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal pprof from response")
	}
	return &p, nil
}
