package main

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/alecthomas/kingpin/v2"
	"github.com/stretchr/testify/require"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func TestQueryFunctions(t *testing.T) {
	t.Parallel()
	const profileID = "550e8400-e29b-41d4-a716-446655440000"
	const traceID = "0123456789abcdef0123456789abcdef"
	for _, tc := range []struct {
		name       string
		flags      []string
		maxNodes   *int64
		profileIDs []string
		traceIDs   []string
		spanIDs    []string
		stack      *typesv1.StackTraceSelector
	}{
		{name: "server default"},
		{name: "explicit zero", flags: []string{"--max-nodes=0"}},
		{
			name:     "limited profile and call site",
			flags:    []string{"--max-nodes=1", "--profile-id=" + profileID, "--stacktrace-selector=root", "--stacktrace-selector=leaf"},
			maxNodes: new(int64(1)), profileIDs: []string{profileID},
			stack: &typesv1.StackTraceSelector{CallSite: []*typesv1.Location{{Name: "root"}, {Name: "leaf"}}},
		},
		{
			name:     "unlimited spans",
			flags:    []string{"--max-nodes=-1", "--span-selector=0000000000000001", "--span-selector=0000000000000002"},
			maxNodes: new(int64(-1)), spanIDs: []string{"0000000000000001", "0000000000000002"},
		},
		{name: "trace", flags: []string{"--trace-id=" + traceID}, traceIDs: []string{traceID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(connect.NewUnaryHandler(
				querierv1connect.QuerierServiceSelectMergeStacktracesProcedure,
				func(_ context.Context, req *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
					want := &querierv1.SelectMergeStacktracesRequest{
						ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
						LabelSelector: `{service_name="test"}`,
						Start:         time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC).UnixMilli(),
						End:           time.Date(2026, 9, 10, 11, 0, 0, 0, time.UTC).UnixMilli(),
						Format:        querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS,
						MaxNodes:      tc.maxNodes, ProfileIdSelector: tc.profileIDs, TraceIdSelector: tc.traceIDs,
						SpanSelector: tc.spanIDs, StackTraceSelector: tc.stack,
					}
					require.True(t, want.EqualVT(req.Msg), "unexpected request: %v", req.Msg)
					require.Equal(t, "test-tenant", req.Header().Get("X-Scope-OrgID"))
					return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{
						Functions: &querierv1.FunctionTable{Total: 20e9, Functions: []*querierv1.FunctionRow{{Name: "leaf", Self: 3e9, Total: 7e9}}},
					}), nil
				},
			))
			t.Cleanup(server.Close)
			app := kingpin.New("profilecli", "")
			params := addQueryFunctionsParams(app.Command("functions", ""))
			args := []string{
				"functions", "--url=" + server.URL, "--tenant-id=test-tenant",
				"--from=2026-09-10T10:00:00Z", "--to=2026-09-10T11:00:00Z",
				`--query={service_name="test"}`, "--output=json",
			}
			_, err := app.Parse(append(args, tc.flags...))
			require.NoError(t, err)
			var buf bytes.Buffer
			require.NoError(t, queryFunctions(withOutput(context.Background(), &buf), params))
			require.JSONEq(t, `{
				"from":"2026-09-10T10:00:00Z", "to":"2026-09-10T11:00:00Z",
				"profile_type":"process_cpu:cpu:nanoseconds:cpu:nanoseconds",
				"total":20000000000, "functions":[{"name":"leaf","self":3000000000,"total":7000000000}]
			}`, buf.String())
		})
	}
}

func TestQueryFunctions_UnsupportedServer(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		err     error
		message string
	}{
		{name: "v1", err: connect.NewError(connect.CodeUnimplemented, errors.New("functions format is only supported with the v2 query backend")), message: "unimplemented"},
		{name: "older server ignores format", message: "server returned no function table"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(connect.NewUnaryHandler(
				querierv1connect.QuerierServiceSelectMergeStacktracesProcedure,
				func(context.Context, *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
					return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{Flamegraph: new(querierv1.FlameGraph)}), tc.err
				},
			))
			t.Cleanup(server.Close)
			app := kingpin.New("profilecli", "")
			params := addQueryFunctionsParams(app.Command("functions", ""))
			_, err := app.Parse([]string{"functions", "--url=" + server.URL})
			require.NoError(t, err)
			var buf bytes.Buffer
			err = queryFunctions(withOutput(context.Background(), &buf), params)
			require.ErrorContains(t, err, tc.message)
			require.Empty(t, buf.String())
		})
	}
}

func TestOutputFunctions(t *testing.T) {
	t.Parallel()
	profileType := &typesv1.ProfileType{ID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds", SampleUnit: "nanoseconds"}
	t.Run("percentages use full total", func(t *testing.T) {
		var buf bytes.Buffer
		result := &querierv1.FunctionTable{Total: 20e9, Functions: []*querierv1.FunctionRow{{Name: "leaf", Self: 3e9, Total: 7e9}}}
		require.NoError(t, outputFunctions(withOutput(context.Background(), &buf), result, "table", time.Time{}, time.Time{}, profileType))
		for _, want := range []string{"Profile total (nanoseconds): 20s", "leaf", "3s", "7s", "15.00%", "35.00%"} {
			require.Contains(t, buf.String(), want)
		}
	})
	t.Run("zero total", func(t *testing.T) {
		var buf bytes.Buffer
		result := &querierv1.FunctionTable{Functions: []*querierv1.FunctionRow{{Name: "zero"}}}
		require.NoError(t, outputFunctions(withOutput(context.Background(), &buf), result, "table", time.Time{}, time.Time{}, profileType))
		require.Contains(t, buf.String(), "0.00%")
		require.NotContains(t, buf.String(), "NaN")
	})
	t.Run("empty json", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, outputFunctions(withOutput(context.Background(), &buf), new(querierv1.FunctionTable), outputJSON, time.Time{}, time.Time{}, profileType))
		require.Contains(t, buf.String(), `"functions": []`)
		require.Contains(t, buf.String(), `"total": 0`)
	})
	t.Run("json preserves integer precision and zero self", func(t *testing.T) {
		var buf bytes.Buffer
		result := &querierv1.FunctionTable{Total: 9007199254740993, Functions: []*querierv1.FunctionRow{{Name: "root", Total: 9007199254740993}}}
		require.NoError(t, outputFunctions(withOutput(context.Background(), &buf), result, outputJSON, time.Time{}, time.Time{}, profileType))
		require.Contains(t, buf.String(), `"total": 9007199254740993`)
		require.Contains(t, buf.String(), `"self": 0`)
	})
}
