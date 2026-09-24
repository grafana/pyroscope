package queryfrontend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/anomalyapi"
	"github.com/grafana/pyroscope/v2/pkg/frontend"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockfrontend"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockmetastorev1"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockqueryfrontend"
)

func TestQueryAnomalies_Unconfigured(t *testing.T) {
	qf := NewQueryFrontend(log.NewNopLogger(), mockfrontend.NewMockLimits(t), frontend.Config{},
		new(mockmetastorev1.MockMetadataQueryServiceClient), nil, new(mockqueryfrontend.MockQueryBackend), nil, nil, nil)

	ctx := tenant.InjectTenantID(context.Background(), smpTenant)
	resp, err := qf.QueryAnomalies(ctx, connect.NewRequest(&querierv1.QueryAnomaliesRequest{
		AnomalyTypes:  []querierv1.AnomalyType{querierv1.AnomalyType_ANOMALY_TYPE_STACKTRACE},
		LabelSelector: `{service_name="svc-a"}`,
	}))

	require.Nil(t, resp)
	require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

func TestQueryAnomalies_NoAnomalyTypes(t *testing.T) {
	qf := NewQueryFrontend(log.NewNopLogger(), mockfrontend.NewMockLimits(t), frontend.Config{},
		new(mockmetastorev1.MockMetadataQueryServiceClient), nil, new(mockqueryfrontend.MockQueryBackend), nil, nil, nil)

	ctx := tenant.InjectTenantID(context.Background(), smpTenant)
	resp, err := qf.QueryAnomalies(ctx, connect.NewRequest(&querierv1.QueryAnomaliesRequest{
		LabelSelector: `{service_name="svc-a"}`,
	}))

	require.Nil(t, resp)
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestQueryAnomalies_ClampsStartToMaxQueryLookback guards against the request's raw Start
// reaching the anomaly source unmodified: resolveServiceNames' own Series() call validates a
// separate copy of the range and doesn't feed the clamped value back, so only the explicit
// SanitizeTimeRange call on req itself makes the anomaly source see the clamped start.
func TestQueryAnomalies_ClampsStartToMaxQueryLookback(t *testing.T) {
	var gotStart string
	apServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotStart = r.URL.Query().Get("start")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"anomalies":[]}`))
	}))
	defer apServer.Close()

	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("MaxQueryLookback", smpTenant).Return(time.Hour)
	mockLimits.On("MaxQueryLength", smpTenant).Return(2 * time.Hour)
	mockLimits.On("QuerySanitizeOnMerge", smpTenant).Return(false)

	mockMetadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	mockMetadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)

	mockBackend := mockqueryfrontend.NewMockQueryBackend(t)
	mockBackend.On("Invoke", mock.Anything, mock.Anything).Return(
		func(ctx context.Context, req *queryv1.InvokeRequest) *queryv1.InvokeResponse {
			if req.Query[0].QueryType == queryv1.QueryType_QUERY_SERIES_LABELS {
				return &queryv1.InvokeResponse{Reports: []*queryv1.Report{{
					ReportType: queryv1.ReportType_REPORT_SERIES_LABELS,
					SeriesLabels: &queryv1.SeriesLabelsReport{
						SeriesLabels: []*typesv1.Labels{{
							Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "svc-a"}},
						}},
					},
				}}}
			}
			return &queryv1.InvokeResponse{Reports: []*queryv1.Report{{
				ReportType:      queryv1.ReportType_REPORT_PROFILE_PRESENCE,
				ProfilePresence: &queryv1.ProfilePresenceReport{},
			}}}
		},
		nil,
	)

	qf := NewQueryFrontend(
		log.NewNopLogger(),
		mockLimits,
		frontend.Config{AnomalyAPI: anomalyapi.Config{URL: apServer.URL}},
		mockMetadata,
		nil,
		mockBackend,
		nil, nil, nil,
	)

	ctx := tenant.InjectTenantID(context.Background(), smpTenant)
	now := time.Now()
	resp, err := qf.QueryAnomalies(ctx, connect.NewRequest(&querierv1.QueryAnomaliesRequest{
		ProfileTypeID: smpProfileType,
		LabelSelector: `{service_name="svc-a"}`,
		Start:         now.Add(-48 * time.Hour).UnixMilli(),
		End:           now.UnixMilli(),
		AnomalyTypes:  []querierv1.AnomalyType{querierv1.AnomalyType_ANOMALY_TYPE_STACKTRACE},
	}))

	require.NoError(t, err)
	require.NotNil(t, resp)

	gotStartMs, err := strconv.ParseInt(gotStart, 10, 64)
	require.NoError(t, err)
	minAllowedStart := now.Add(-time.Hour).UnixMilli()
	require.GreaterOrEqual(t, gotStartMs, minAllowedStart-1000,
		"anomaly source received a start before the tenant's max_query_lookback boundary")
}

// TestQueryAnomalies_TimeRangeBeforeLookback_ReturnsEmpty: a range fully outside
// max_query_lookback must return an empty result without reaching the anomaly source or the
// query-backend at all (no mock expectations are set on either).
func TestQueryAnomalies_TimeRangeBeforeLookback_ReturnsEmpty(t *testing.T) {
	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("MaxQueryLookback", smpTenant).Return(time.Hour)

	qf := NewQueryFrontend(
		log.NewNopLogger(),
		mockLimits,
		frontend.Config{AnomalyAPI: anomalyapi.Config{URL: "http://unused"}},
		new(mockmetastorev1.MockMetadataQueryServiceClient),
		nil,
		new(mockqueryfrontend.MockQueryBackend),
		nil, nil, nil,
	)

	ctx := tenant.InjectTenantID(context.Background(), smpTenant)
	now := time.Now()
	resp, err := qf.QueryAnomalies(ctx, connect.NewRequest(&querierv1.QueryAnomaliesRequest{
		LabelSelector: `{service_name="svc-a"}`,
		Start:         now.Add(-48 * time.Hour).UnixMilli(),
		End:           now.Add(-24 * time.Hour).UnixMilli(),
		AnomalyTypes:  []querierv1.AnomalyType{querierv1.AnomalyType_ANOMALY_TYPE_STACKTRACE},
	}))

	require.NoError(t, err)
	require.Empty(t, resp.Msg.StacktraceAnomalies)
}

func TestQueryAnomalies_UnknownType(t *testing.T) {
	qf := NewQueryFrontend(log.NewNopLogger(), mockfrontend.NewMockLimits(t), frontend.Config{},
		new(mockmetastorev1.MockMetadataQueryServiceClient), nil, new(mockqueryfrontend.MockQueryBackend), nil, nil, nil)

	ctx := tenant.InjectTenantID(context.Background(), smpTenant)
	resp, err := qf.QueryAnomalies(ctx, connect.NewRequest(&querierv1.QueryAnomaliesRequest{
		AnomalyTypes:  []querierv1.AnomalyType{querierv1.AnomalyType(99)},
		LabelSelector: `{service_name="svc-a"}`,
	}))

	require.Nil(t, resp)
	require.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}

func TestQueryAnomalies_NoMatchingServiceName(t *testing.T) {
	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
	mockLimits.On("QuerySanitizeOnMerge", smpTenant).Return(false)

	mockMetadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	mockMetadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)

	mockBackend := mockqueryfrontend.NewMockQueryBackend(t)
	mockBackend.On("Invoke", mock.Anything, mock.Anything).Return(&queryv1.InvokeResponse{Reports: []*queryv1.Report{{
		ReportType:   queryv1.ReportType_REPORT_SERIES_LABELS,
		SeriesLabels: &queryv1.SeriesLabelsReport{},
	}}}, nil)

	qf := NewQueryFrontend(
		log.NewNopLogger(),
		mockLimits,
		frontend.Config{AnomalyAPI: anomalyapi.Config{URL: "http://unused"}},
		mockMetadata,
		nil,
		mockBackend,
		nil,
		nil,
		nil,
	)

	ctx := tenant.InjectTenantID(context.Background(), smpTenant)
	start, end := smpValidTimeRange()
	resp, err := qf.QueryAnomalies(ctx, connect.NewRequest(&querierv1.QueryAnomaliesRequest{
		ProfileTypeID: smpProfileType,
		LabelSelector: `{namespace="empty-namespace"}`,
		Start:         start,
		End:           end,
		AnomalyTypes:  []querierv1.AnomalyType{querierv1.AnomalyType_ANOMALY_TYPE_STACKTRACE},
	}))

	require.Nil(t, resp)
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestQueryAnomalies_MultipleServiceNames(t *testing.T) {
	// A namespace-wide selector resolves to two services; the anomaly source is called once
	// with both, and anomalies confirmed present from either service come back.
	apServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.ElementsMatch(t, []string{"svc-a", "svc-b"}, r.URL.Query()["service_name"])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"anomalies":[
			{"profile_uuid":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","score":-0.9,"observed_at":"2026-09-23T12:00:00Z","model_id":"m1"},
			{"profile_uuid":"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb","score":-0.7,"observed_at":"2026-09-23T12:01:00Z","model_id":"m1"}
		]}`))
	}))
	defer apServer.Close()

	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
	mockLimits.On("QuerySanitizeOnMerge", smpTenant).Return(false)

	mockMetadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	mockMetadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)

	mockBackend := mockqueryfrontend.NewMockQueryBackend(t)
	mockBackend.On("Invoke", mock.Anything, mock.Anything).Return(
		func(ctx context.Context, req *queryv1.InvokeRequest) *queryv1.InvokeResponse {
			if req.Query[0].QueryType == queryv1.QueryType_QUERY_SERIES_LABELS {
				return &queryv1.InvokeResponse{Reports: []*queryv1.Report{{
					ReportType: queryv1.ReportType_REPORT_SERIES_LABELS,
					SeriesLabels: &queryv1.SeriesLabelsReport{
						SeriesLabels: []*typesv1.Labels{
							{Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "svc-a"}}},
							{Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "svc-b"}}},
						},
					},
				}}}
			}
			candidates := req.Query[0].ProfilePresence.GetProfileIdSelector()
			present := make([]*queryv1.ProfilePresenceEntry, len(candidates))
			for i, id := range candidates {
				present[i] = &queryv1.ProfilePresenceEntry{ProfileId: id}
			}
			return &queryv1.InvokeResponse{Reports: []*queryv1.Report{{
				ReportType:      queryv1.ReportType_REPORT_PROFILE_PRESENCE,
				ProfilePresence: &queryv1.ProfilePresenceReport{Profiles: present},
			}}}
		},
		nil,
	)

	qf := NewQueryFrontend(
		log.NewNopLogger(),
		mockLimits,
		frontend.Config{AnomalyAPI: anomalyapi.Config{URL: apServer.URL}},
		mockMetadata,
		nil,
		mockBackend,
		nil,
		nil,
		nil,
	)

	ctx := tenant.InjectTenantID(context.Background(), smpTenant)
	start, end := smpValidTimeRange()
	resp, err := qf.QueryAnomalies(ctx, connect.NewRequest(&querierv1.QueryAnomaliesRequest{
		ProfileTypeID: smpProfileType,
		LabelSelector: `{namespace="shared-namespace"}`,
		Start:         start,
		End:           end,
		AnomalyTypes:  []querierv1.AnomalyType{querierv1.AnomalyType_ANOMALY_TYPE_STACKTRACE},
	}))

	require.NoError(t, err)
	gotIDs := make([]string, len(resp.Msg.StacktraceAnomalies))
	for i, p := range resp.Msg.StacktraceAnomalies {
		gotIDs[i] = p.ProfileId
	}
	require.ElementsMatch(t, []string{"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"}, gotIDs)
}

func TestQueryAnomalies_StacktraceConfirmsAgainstIngestedData(t *testing.T) {
	// The anomaly source reports three anomalies; the mock backend confirms only 11111111-1111-1111-1111-111111111111
	// and 22222222-2222-2222-2222-222222222222, in one call covering all three.
	apServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/anomalydetection/anomalies", r.URL.Path)
		require.Equal(t, smpTenant, r.Header.Get("X-Scope-OrgID"))
		require.Equal(t, "svc-a", r.URL.Query().Get("service_name"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"anomalies":[
			{"profile_uuid":"11111111-1111-1111-1111-111111111111","score":-0.9,"observed_at":"2026-09-23T12:00:00Z","model_id":"m1"},
			{"profile_uuid":"00000000-0000-0000-0000-000000000001","score":-0.8,"observed_at":"2026-09-23T12:01:00Z","model_id":"m1"},
			{"profile_uuid":"22222222-2222-2222-2222-222222222222","score":-0.7,"observed_at":"2026-09-23T12:02:00Z","model_id":"m1"}
		]}`))
	}))
	defer apServer.Close()

	backendTimestampByID := map[string]int64{"11111111-1111-1111-1111-111111111111": 111, "22222222-2222-2222-2222-222222222222": 222}

	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
	mockLimits.On("QuerySanitizeOnMerge", smpTenant).Return(false)

	mockMetadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	mockMetadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)

	var invokeCalls atomic.Int32
	mockBackend := mockqueryfrontend.NewMockQueryBackend(t)
	mockBackend.On("Invoke", mock.Anything, mock.Anything).Return(
		func(ctx context.Context, req *queryv1.InvokeRequest) *queryv1.InvokeResponse {
			invokeCalls.Add(1)
			switch req.Query[0].QueryType {
			case queryv1.QueryType_QUERY_SERIES_LABELS:
				return &queryv1.InvokeResponse{Reports: []*queryv1.Report{{
					ReportType: queryv1.ReportType_REPORT_SERIES_LABELS,
					SeriesLabels: &queryv1.SeriesLabelsReport{
						SeriesLabels: []*typesv1.Labels{{
							Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "svc-a"}},
						}},
					},
				}}}
			default:
				candidates := req.Query[0].ProfilePresence.GetProfileIdSelector()
				present := make([]*queryv1.ProfilePresenceEntry, 0, len(candidates))
				for _, id := range candidates {
					if id != "00000000-0000-0000-0000-000000000001" {
						present = append(present, &queryv1.ProfilePresenceEntry{
							ProfileId: id,
							Labels:    []*typesv1.LabelPair{{Name: "pod", Value: id + "-pod"}},
							Timestamp: backendTimestampByID[id],
						})
					}
				}
				return &queryv1.InvokeResponse{Reports: []*queryv1.Report{{
					ReportType:      queryv1.ReportType_REPORT_PROFILE_PRESENCE,
					ProfilePresence: &queryv1.ProfilePresenceReport{Profiles: present},
				}}}
			}
		},
		nil,
	)

	qf := NewQueryFrontend(
		log.NewNopLogger(),
		mockLimits,
		frontend.Config{AnomalyAPI: anomalyapi.Config{URL: apServer.URL}},
		mockMetadata,
		nil,
		mockBackend,
		nil,
		nil,
		nil,
	)

	ctx := tenant.InjectTenantID(context.Background(), smpTenant)
	start, end := smpValidTimeRange()
	resp, err := qf.QueryAnomalies(ctx, connect.NewRequest(&querierv1.QueryAnomaliesRequest{
		ProfileTypeID: smpProfileType,
		LabelSelector: `{service_name="svc-a"}`,
		Start:         start,
		End:           end,
		AnomalyTypes:  []querierv1.AnomalyType{querierv1.AnomalyType_ANOMALY_TYPE_STACKTRACE},
	}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.StacktraceAnomalies, 2)
	byID := make(map[string]*querierv1.StacktraceAnomaly, len(resp.Msg.StacktraceAnomalies))
	for _, p := range resp.Msg.StacktraceAnomalies {
		byID[p.ProfileId] = p
	}
	require.Equal(t, backendTimestampByID["11111111-1111-1111-1111-111111111111"], byID["11111111-1111-1111-1111-111111111111"].Timestamp)
	require.Equal(t, backendTimestampByID["22222222-2222-2222-2222-222222222222"], byID["22222222-2222-2222-2222-222222222222"].Timestamp)
	require.Equal(t, -0.9, byID["11111111-1111-1111-1111-111111111111"].Score)
	require.Equal(t, -0.7, byID["22222222-2222-2222-2222-222222222222"].Score)
	require.Equal(t, []*typesv1.LabelPair{{Name: "pod", Value: "11111111-1111-1111-1111-111111111111-pod"}}, byID["11111111-1111-1111-1111-111111111111"].Labels)
	require.Equal(t, []*typesv1.LabelPair{{Name: "pod", Value: "22222222-2222-2222-2222-222222222222-pod"}}, byID["22222222-2222-2222-2222-222222222222"].Labels)
	require.EqualValues(t, 2, invokeCalls.Load())
}

func TestQueryAnomalies_AllAbsent_SingleBackendCall(t *testing.T) {
	// None of the anomaly source's anomalies are actually present. Confirms the whole
	// candidate batch resolves via a single query-backend call, regardless of candidate count.
	apServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"anomalies":[
			{"profile_uuid":"00000000-0000-0000-0000-000000000001","score":-0.9,"observed_at":"2026-09-23T12:00:00Z","model_id":"m1"},
			{"profile_uuid":"00000000-0000-0000-0000-000000000002","score":-0.8,"observed_at":"2026-09-23T12:01:00Z","model_id":"m1"},
			{"profile_uuid":"00000000-0000-0000-0000-000000000003","score":-0.7,"observed_at":"2026-09-23T12:02:00Z","model_id":"m1"}
		]}`))
	}))
	defer apServer.Close()

	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
	mockLimits.On("QuerySanitizeOnMerge", smpTenant).Return(false)

	mockMetadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	mockMetadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)

	var invokeCalls atomic.Int32
	mockBackend := mockqueryfrontend.NewMockQueryBackend(t)
	mockBackend.On("Invoke", mock.Anything, mock.Anything).Return(
		func(ctx context.Context, req *queryv1.InvokeRequest) *queryv1.InvokeResponse {
			invokeCalls.Add(1)
			if req.Query[0].QueryType == queryv1.QueryType_QUERY_SERIES_LABELS {
				return &queryv1.InvokeResponse{Reports: []*queryv1.Report{{
					ReportType: queryv1.ReportType_REPORT_SERIES_LABELS,
					SeriesLabels: &queryv1.SeriesLabelsReport{
						SeriesLabels: []*typesv1.Labels{{
							Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "svc-a"}},
						}},
					},
				}}}
			}
			return &queryv1.InvokeResponse{Reports: []*queryv1.Report{{
				ReportType:      queryv1.ReportType_REPORT_PROFILE_PRESENCE,
				ProfilePresence: &queryv1.ProfilePresenceReport{},
			}}}
		},
		nil,
	)

	qf := NewQueryFrontend(
		log.NewNopLogger(),
		mockLimits,
		frontend.Config{AnomalyAPI: anomalyapi.Config{URL: apServer.URL}},
		mockMetadata,
		nil,
		mockBackend,
		nil,
		nil,
		nil,
	)

	ctx := tenant.InjectTenantID(context.Background(), smpTenant)
	start, end := smpValidTimeRange()
	resp, err := qf.QueryAnomalies(ctx, connect.NewRequest(&querierv1.QueryAnomaliesRequest{
		ProfileTypeID: smpProfileType,
		LabelSelector: `{service_name="svc-a"}`,
		Start:         start,
		End:           end,
		AnomalyTypes:  []querierv1.AnomalyType{querierv1.AnomalyType_ANOMALY_TYPE_STACKTRACE},
	}))

	require.NoError(t, err)
	require.Empty(t, resp.Msg.StacktraceAnomalies)
	require.EqualValues(t, 2, invokeCalls.Load())
}

// TestQueryAnomalies_ScoreLookupToleratesUUIDSpelling: profile presence matches UUIDs by their
// parsed byte value, so the confirmed ProfileId comes back in canonical form even when the
// anomaly source's spelling differs. The score lookup must key off the same canonical form,
// not the anomaly source's raw spelling, or it silently attaches a zero score.
func TestQueryAnomalies_ScoreLookupToleratesUUIDSpelling(t *testing.T) {
	const rawSpelling = "AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA"
	const canonicalSpelling = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

	apServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"anomalies":[
			{"profile_uuid":"` + rawSpelling + `","score":-0.42,"observed_at":"2026-09-23T12:00:00Z","model_id":"m1"}
		]}`))
	}))
	defer apServer.Close()

	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
	mockLimits.On("QuerySanitizeOnMerge", smpTenant).Return(false)

	mockMetadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	mockMetadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)

	mockBackend := mockqueryfrontend.NewMockQueryBackend(t)
	mockBackend.On("Invoke", mock.Anything, mock.Anything).Return(
		func(ctx context.Context, req *queryv1.InvokeRequest) *queryv1.InvokeResponse {
			if req.Query[0].QueryType == queryv1.QueryType_QUERY_SERIES_LABELS {
				return &queryv1.InvokeResponse{Reports: []*queryv1.Report{{
					ReportType: queryv1.ReportType_REPORT_SERIES_LABELS,
					SeriesLabels: &queryv1.SeriesLabelsReport{
						SeriesLabels: []*typesv1.Labels{{
							Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "svc-a"}},
						}},
					},
				}}}
			}
			candidates := req.Query[0].ProfilePresence.GetProfileIdSelector()
			present := make([]*queryv1.ProfilePresenceEntry, len(candidates))
			for i, id := range candidates {
				u, err := uuid.Parse(id)
				require.NoError(t, err)
				present[i] = &queryv1.ProfilePresenceEntry{ProfileId: u.String()}
			}
			return &queryv1.InvokeResponse{Reports: []*queryv1.Report{{
				ReportType:      queryv1.ReportType_REPORT_PROFILE_PRESENCE,
				ProfilePresence: &queryv1.ProfilePresenceReport{Profiles: present},
			}}}
		},
		nil,
	)

	qf := NewQueryFrontend(
		log.NewNopLogger(),
		mockLimits,
		frontend.Config{AnomalyAPI: anomalyapi.Config{URL: apServer.URL}},
		mockMetadata,
		nil,
		mockBackend,
		nil, nil, nil,
	)

	ctx := tenant.InjectTenantID(context.Background(), smpTenant)
	start, end := smpValidTimeRange()
	resp, err := qf.QueryAnomalies(ctx, connect.NewRequest(&querierv1.QueryAnomaliesRequest{
		ProfileTypeID: smpProfileType,
		LabelSelector: `{service_name="svc-a"}`,
		Start:         start,
		End:           end,
		AnomalyTypes:  []querierv1.AnomalyType{querierv1.AnomalyType_ANOMALY_TYPE_STACKTRACE},
	}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.StacktraceAnomalies, 1)
	require.Equal(t, canonicalSpelling, resp.Msg.StacktraceAnomalies[0].ProfileId)
	require.Equal(t, -0.42, resp.Msg.StacktraceAnomalies[0].Score)
}
