package querier

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func Test_ParseQuery(t *testing.T) {
	q := url.Values{
		"query": []string{`memory:alloc_space:bytes:space:bytes{foo="bar",bar=~"buzz"}`},
		"from":  []string{"now-6h"},
		"until": []string{"now"},
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost/render/render?%s", q.Encode()), nil)
	require.NoError(t, err)
	require.NoError(t, req.ParseForm())

	queryRequest, ptype, err := parseSelectProfilesRequest(renderRequestFieldNames{}, req)
	require.NoError(t, err)
	require.WithinDuration(t, time.Now(), model.Time(queryRequest.End).Time(), 1*time.Minute)
	require.WithinDuration(t, time.Now().Add(-6*time.Hour), model.Time(queryRequest.Start).Time(), 1*time.Minute)

	require.Equal(t, &typesv1.ProfileType{
		ID:         "memory:alloc_space:bytes:space:bytes",
		Name:       "memory",
		SampleType: "alloc_space",
		SampleUnit: "bytes",
		PeriodType: "space",
		PeriodUnit: "bytes",
	}, ptype)

	require.Equal(t, `{foo="bar",bar=~"buzz"}`, queryRequest.LabelSelector)
}

func Test_ParseSelectProfilesRequest_DefaultFromUntil(t *testing.T) {
	tests := []struct {
		name          string
		queryParams   url.Values
		expectedStart time.Time
		expectedEnd   time.Time
	}{
		{
			name: "both from and until missing defaults to now",
			queryParams: url.Values{
				"query": []string{`memory:alloc_space:bytes:space:bytes{}`},
			},
			expectedStart: time.Now(),
			expectedEnd:   time.Now(),
		},
		{
			name: "from missing defaults to now",
			queryParams: url.Values{
				"query": []string{`memory:alloc_space:bytes:space:bytes{}`},
				"until": []string{"now-1h"},
			},
			expectedStart: time.Now(),
			expectedEnd:   time.Now().Add(-1 * time.Hour),
		},
		{
			name: "until missing defaults to now",
			queryParams: url.Values{
				"query": []string{`memory:alloc_space:bytes:space:bytes{}`},
				"from":  []string{"now-6h"},
			},
			expectedStart: time.Now().Add(-6 * time.Hour),
			expectedEnd:   time.Now(),
		},
		{
			name: "both provided uses provided values",
			queryParams: url.Values{
				"query": []string{`memory:alloc_space:bytes:space:bytes{}`},
				"from":  []string{"now-6h"},
				"until": []string{"now-1h"},
			},
			expectedStart: time.Now().Add(-6 * time.Hour),
			expectedEnd:   time.Now().Add(-1 * time.Hour),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost/render/render?%s", tt.queryParams.Encode()), nil)
			require.NoError(t, err)
			require.NoError(t, req.ParseForm())

			queryRequest, _, err := parseSelectProfilesRequest(renderRequestFieldNames{}, req)
			require.NoError(t, err)

			require.WithinDuration(t, tt.expectedStart, model.Time(queryRequest.Start).Time(), 1*time.Minute)
			require.WithinDuration(t, tt.expectedEnd, model.Time(queryRequest.End).Time(), 1*time.Minute)
		})
	}
}

// mockQuerierClient is a mock implementation of QuerierServiceClient
type mockQuerierClient struct {
	selectMergeProfileFunc func(context.Context, *connect.Request[querierv1.SelectMergeProfileRequest]) (*connect.Response[profilev1.Profile], error)
}

func (m *mockQuerierClient) ProfileTypes(context.Context, *connect.Request[querierv1.ProfileTypesRequest]) (*connect.Response[querierv1.ProfileTypesResponse], error) {
	return nil, nil
}

func (m *mockQuerierClient) LabelValues(context.Context, *connect.Request[typesv1.LabelValuesRequest]) (*connect.Response[typesv1.LabelValuesResponse], error) {
	return nil, nil
}

func (m *mockQuerierClient) LabelNames(context.Context, *connect.Request[typesv1.LabelNamesRequest]) (*connect.Response[typesv1.LabelNamesResponse], error) {
	return nil, nil
}

func (m *mockQuerierClient) Series(context.Context, *connect.Request[querierv1.SeriesRequest]) (*connect.Response[querierv1.SeriesResponse], error) {
	return nil, nil
}

func (m *mockQuerierClient) SelectMergeStacktraces(context.Context, *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	return nil, nil
}

func (m *mockQuerierClient) SelectMergeSpanProfile(context.Context, *connect.Request[querierv1.SelectMergeSpanProfileRequest]) (*connect.Response[querierv1.SelectMergeSpanProfileResponse], error) {
	return nil, nil
}

func (m *mockQuerierClient) SelectMergeProfile(ctx context.Context, req *connect.Request[querierv1.SelectMergeProfileRequest]) (*connect.Response[profilev1.Profile], error) {
	if m.selectMergeProfileFunc != nil {
		return m.selectMergeProfileFunc(ctx, req)
	}
	return nil, nil
}

func (m *mockQuerierClient) SelectSeries(context.Context, *connect.Request[querierv1.SelectSeriesRequest]) (*connect.Response[querierv1.SelectSeriesResponse], error) {
	return nil, nil
}

func (m *mockQuerierClient) Diff(context.Context, *connect.Request[querierv1.DiffRequest]) (*connect.Response[querierv1.DiffResponse], error) {
	return nil, nil
}

func (m *mockQuerierClient) GetProfileStats(context.Context, *connect.Request[typesv1.GetProfileStatsRequest]) (*connect.Response[typesv1.GetProfileStatsResponse], error) {
	return nil, nil
}

func (m *mockQuerierClient) AnalyzeQuery(context.Context, *connect.Request[querierv1.AnalyzeQueryRequest]) (*connect.Response[querierv1.AnalyzeQueryResponse], error) {
	return nil, nil
}

func Test_RenderDotFormatEmptyProfile(t *testing.T) {
	// Create a mock client that returns an empty profile
	mockClient := &mockQuerierClient{
		selectMergeProfileFunc: func(ctx context.Context, req *connect.Request[querierv1.SelectMergeProfileRequest]) (*connect.Response[profilev1.Profile], error) {
			// Return an empty profile (no samples)
			return connect.NewResponse(&profilev1.Profile{
				Sample: []*profilev1.Sample{}, // Empty samples
			}), nil
		},
	}

	handlers := NewHTTPHandlers(mockClient)

	// Create a request with format=dot
	q := url.Values{
		"query":  []string{`memory:alloc_space:bytes:space:bytes{}`},
		"from":   []string{"now-1h"},
		"until":  []string{"now"},
		"format": []string{"dot"},
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost/render?%s", q.Encode()), nil)
	require.NoError(t, err)

	// Create a response recorder
	rr := httptest.NewRecorder()

	// Call the handler
	handlers.Render(rr, req)

	// Verify we get a 200 OK with empty body instead of 500 (Internal Server Error)
	require.Equal(t, http.StatusOK, rr.Code, "Expected 200 OK for empty profile, got %d", rr.Code)
	require.Equal(t, "", rr.Body.String(), "Expected empty body for empty profile")
	require.Equal(t, "text/plain", rr.Header().Get("Content-Type"), "Expected text/plain content type")
}
