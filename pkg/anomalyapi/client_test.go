package anomalyapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNew_EmptyURLDisables(t *testing.T) {
	require.Nil(t, New(Config{}, nil))
}

func TestListAnomalies(t *testing.T) {
	var gotTenantHeader, gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTenantHeader = r.Header.Get(TenantHeader)
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"anomalies":[
			{"profile_uuid":"abc-1","score":-0.42,"observed_at":"2026-09-23T12:00:00Z","model_id":"5fm4k"},
			{"profile_uuid":"abc-2","score":-0.13,"observed_at":"2026-09-23T12:01:00Z","model_id":"5fm4k"}
		]}`))
	}))
	defer server.Close()

	client := New(Config{URL: server.URL}, nil)
	require.NotNil(t, client)

	start := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 23, 12, 5, 0, 0, time.UTC)

	anomalies, err := client.ListAnomalies(context.Background(), "tenant-1", []string{"svc-a", "svc-b"}, start, end)
	require.NoError(t, err)
	require.Len(t, anomalies, 2)
	require.Equal(t, "abc-1", anomalies[0].ProfileUUID)
	require.Equal(t, -0.42, anomalies[0].Score)
	require.Equal(t, "5fm4k", anomalies[0].ModelID)

	require.Equal(t, "tenant-1", gotTenantHeader)
	require.Equal(t, "/api/v1/anomalydetection/anomalies", gotPath)
	require.Contains(t, gotQuery, "service_name=svc-a")
	require.Contains(t, gotQuery, "service_name=svc-b")
	require.Contains(t, gotQuery, "start=1790164800000")
	require.Contains(t, gotQuery, "end=1790165100000")
}

func TestListAnomalies_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := New(Config{URL: server.URL}, nil)
	_, err := client.ListAnomalies(context.Background(), "tenant-1", []string{"svc-a"}, time.Now(), time.Now())
	require.Error(t, err)
}

// TestListAnomalies_TooManyServices guards against a broad or empty selector resolving to
// enough services that repeated service_name params alone could blow past a proxy's URL
// length limit: ListAnomalies must reject it up front rather than fire an oversized request.
func TestListAnomalies_TooManyServices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("anomaly source should not be called when the service list exceeds the limit")
	}))
	defer server.Close()

	client := New(Config{URL: server.URL}, nil)
	require.NotNil(t, client)

	serviceNames := make([]string, maxServiceNames+1)
	for i := range serviceNames {
		serviceNames[i] = fmt.Sprintf("svc-%d", i)
	}

	_, err := client.ListAnomalies(context.Background(), "tenant-1", serviceNames, time.Now(), time.Now())
	require.Error(t, err)
}
