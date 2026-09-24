package queryfrontend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
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
		AnomalyType:   "stacktrace",
		LabelSelector: `{service_name="svc-a"}`,
	}))

	require.Nil(t, resp)
	require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

func TestQueryAnomalies_UnknownType(t *testing.T) {
	qf := NewQueryFrontend(log.NewNopLogger(), mockfrontend.NewMockLimits(t), frontend.Config{},
		new(mockmetastorev1.MockMetadataQueryServiceClient), nil, new(mockqueryfrontend.MockQueryBackend), nil, nil, nil)

	ctx := tenant.InjectTenantID(context.Background(), smpTenant)
	resp, err := qf.QueryAnomalies(ctx, connect.NewRequest(&querierv1.QueryAnomaliesRequest{
		AnomalyType:   "some-other-type",
		LabelSelector: `{service_name="svc-a"}`,
	}))

	require.Nil(t, resp)
	require.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}

func TestQueryAnomalies_StacktraceConfirmsAgainstIngestedData(t *testing.T) {
	// The anomaly source reports three anomalies; the mock backend confirms only present-1
	// and present-2, in one call covering all three.
	apServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/anomalydetection/anomalies", r.URL.Path)
		require.Equal(t, smpTenant, r.Header.Get("X-Scope-OrgID"))
		require.Equal(t, "svc-a", r.URL.Query().Get("service_name"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"anomalies":[
			{"profile_uuid":"present-1","score":-0.9,"observed_at":"2026-09-23T12:00:00Z","model_id":"m1"},
			{"profile_uuid":"absent-1","score":-0.8,"observed_at":"2026-09-23T12:01:00Z","model_id":"m1"},
			{"profile_uuid":"present-2","score":-0.7,"observed_at":"2026-09-23T12:02:00Z","model_id":"m1"}
		]}`))
	}))
	defer apServer.Close()

	backendTimestampByID := map[string]int64{"present-1": 111, "present-2": 222}

	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("QuerySanitizeOnMerge", smpTenant).Return(false)

	mockMetadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	mockMetadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)

	var invokeCalls atomic.Int32
	mockBackend := mockqueryfrontend.NewMockQueryBackend(t)
	mockBackend.On("Invoke", mock.Anything, mock.Anything).Return(
		func(ctx context.Context, req *queryv1.InvokeRequest) *queryv1.InvokeResponse {
			invokeCalls.Add(1)
			candidates := req.Query[0].ProfilePresence.GetProfileIdSelector()
			present := make([]*queryv1.ProfilePresenceEntry, 0, len(candidates))
			for _, id := range candidates {
				if id != "absent-1" {
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
		AnomalyType:   "stacktrace",
	}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.Profiles, 2)
	byID := make(map[string]*querierv1.StacktraceAnomaly, len(resp.Msg.Profiles))
	for _, p := range resp.Msg.Profiles {
		byID[p.ProfileId] = p
	}
	require.Equal(t, backendTimestampByID["present-1"], byID["present-1"].Timestamp)
	require.Equal(t, backendTimestampByID["present-2"], byID["present-2"].Timestamp)
	require.Equal(t, -0.9, byID["present-1"].Score)
	require.Equal(t, -0.7, byID["present-2"].Score)
	require.Equal(t, []*typesv1.LabelPair{{Name: "pod", Value: "present-1-pod"}}, byID["present-1"].Labels)
	require.Equal(t, []*typesv1.LabelPair{{Name: "pod", Value: "present-2-pod"}}, byID["present-2"].Labels)
	require.EqualValues(t, 1, invokeCalls.Load())
}

func TestQueryAnomalies_AllAbsent_SingleBackendCall(t *testing.T) {
	// None of the anomaly source's anomalies are actually present. Confirms the whole
	// candidate batch resolves via a single query-backend call, regardless of candidate count.
	apServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"anomalies":[
			{"profile_uuid":"absent-1","score":-0.9,"observed_at":"2026-09-23T12:00:00Z","model_id":"m1"},
			{"profile_uuid":"absent-2","score":-0.8,"observed_at":"2026-09-23T12:01:00Z","model_id":"m1"},
			{"profile_uuid":"absent-3","score":-0.7,"observed_at":"2026-09-23T12:02:00Z","model_id":"m1"}
		]}`))
	}))
	defer apServer.Close()

	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("QuerySanitizeOnMerge", smpTenant).Return(false)

	mockMetadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	mockMetadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)

	var invokeCalls atomic.Int32
	mockBackend := mockqueryfrontend.NewMockQueryBackend(t)
	mockBackend.On("Invoke", mock.Anything, mock.Anything).Run(func(mock.Arguments) {
		invokeCalls.Add(1)
	}).Return(&queryv1.InvokeResponse{Reports: []*queryv1.Report{{
		ReportType:      queryv1.ReportType_REPORT_PROFILE_PRESENCE,
		ProfilePresence: &queryv1.ProfilePresenceReport{},
	}}}, nil)

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
		AnomalyType:   "stacktrace",
	}))

	require.NoError(t, err)
	require.Empty(t, resp.Msg.Profiles)
	require.EqualValues(t, 1, invokeCalls.Load())
}
