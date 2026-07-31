package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	pprofprofile "github.com/google/pprof/profile"
	"github.com/stretchr/testify/require"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	connectapi "github.com/grafana/pyroscope/v2/pkg/api/connect"
)

func TestFirstSeed(t *testing.T) {
	t.Parallel()

	service := &seedTestService{records: make(map[string]seedRecord)}
	mux := http.NewServeMux()
	path, handler := pushv1connect.NewPusherServiceHandler(service, connectapi.DefaultHandlerOptions()...)
	mux.Handle(path, handler)
	path, handler = querierv1connect.NewQuerierServiceHandler(service, connectapi.DefaultHandlerOptions()...)
	mux.Handle(path, handler)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	now := time.Date(2026, time.July, 31, 12, 0, 0, 123456789, time.UTC)
	err := firstSeed(context.Background(), firstSeedConfig{
		writeURL:          server.URL,
		readURL:           server.URL,
		httpClient:        server.Client(),
		now:               func() time.Time { return now },
		visibilityPoll:    time.Millisecond,
		visibilityTimeout: time.Second,
	})
	require.NoError(t, err)

	wantSeedID := strconv.FormatInt(now.Add(-time.Second).Truncate(time.Millisecond).UnixNano(), 10)
	service.mu.Lock()
	defer service.mu.Unlock()
	require.Len(t, service.records, len(seeds))
	for _, s := range seeds {
		record, ok := service.records[s.tenantID]
		require.True(t, ok)
		require.Equal(t, s.marker, record.marker)
		require.Equal(t, wantSeedID, record.seedID)
		require.Equal(t, smokeProfileMaxSamples, record.samples)
		require.Equal(t, now.Add(-time.Second).Truncate(time.Millisecond).UnixNano(), record.profileTime)
		require.True(t, record.queried)
	}
}

func TestBuildSmokeProfileFitsDefaultIngestionLimits(t *testing.T) {
	t.Parallel()

	profileTime := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	data, err := buildSmokeProfile(profileTime)
	require.NoError(t, err)

	profile, err := pprofprofile.ParseData(data)
	require.NoError(t, err)
	require.Len(t, profile.Sample, smokeProfileMaxSamples)
	require.Equal(t, profileTime.UnixNano(), profile.TimeNanos)

	var uncompressed bytes.Buffer
	require.NoError(t, profile.WriteUncompressed(&uncompressed))
	require.Less(t, uncompressed.Len(), 4*1024*1024)
}

type seedRecord struct {
	marker      string
	seedID      string
	samples     int
	profileTime int64
	queried     bool
}

type seedTestService struct {
	pushv1connect.UnimplementedPusherServiceHandler
	querierv1connect.UnimplementedQuerierServiceHandler

	mu      sync.Mutex
	records map[string]seedRecord
}

func (s *seedTestService) Push(
	_ context.Context,
	req *connect.Request[pushv1.PushRequest],
) (*connect.Response[pushv1.PushResponse], error) {
	tenantID := req.Header().Get("X-Scope-OrgID")
	if tenantID == "" || len(req.Msg.Series) != 1 || len(req.Msg.Series[0].Samples) != 1 {
		return nil, connect.NewError(connect.CodeInvalidArgument, nil)
	}

	series := req.Msg.Series[0]
	marker := findLabel(series.Labels, seedMarkerLabel)
	seedID := findLabel(series.Labels, seedIDLabel)
	if marker == "" || seedID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, nil)
	}

	profile, err := pprofprofile.ParseData(series.Samples[0].RawProfile)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if len(profile.Sample) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, nil)
	}

	s.mu.Lock()
	s.records[tenantID] = seedRecord{
		marker:      marker,
		seedID:      seedID,
		samples:     len(profile.Sample),
		profileTime: profile.TimeNanos,
	}
	s.mu.Unlock()
	return connect.NewResponse(&pushv1.PushResponse{}), nil
}

func (s *seedTestService) LabelValues(
	_ context.Context,
	req *connect.Request[typesv1.LabelValuesRequest],
) (*connect.Response[typesv1.LabelValuesResponse], error) {
	tenantID := req.Header().Get("X-Scope-OrgID")
	if tenantID == "" || req.Msg.Name != seedIDLabel {
		return nil, connect.NewError(connect.CodeInvalidArgument, nil)
	}

	s.mu.Lock()
	record, ok := s.records[tenantID]
	if ok {
		record.queried = true
		s.records[tenantID] = record
	}
	s.mu.Unlock()
	if !ok {
		return connect.NewResponse(&typesv1.LabelValuesResponse{}), nil
	}
	return connect.NewResponse(&typesv1.LabelValuesResponse{Names: []string{record.seedID}}), nil
}

func findLabel(labels []*typesv1.LabelPair, name string) string {
	for _, label := range labels {
		if label.Name == name {
			return label.Value
		}
	}
	return ""
}
