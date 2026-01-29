package diagnostics

import (
	"context"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tenant"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

// Wrapper wraps a QuerierServiceClient to handle diagnostics flushing.
// After each request, if diagnostics collection is enabled, it flushes
// the collected diagnostics to the store with the total response time.
type Wrapper struct {
	logger log.Logger
	client querierv1connect.QuerierServiceClient
	store  *Store
}

// NewWrapper creates a new diagnostics wrapper.
func NewWrapper(logger log.Logger, client querierv1connect.QuerierServiceClient, store *Store) *Wrapper {
	return &Wrapper{
		logger: logger,
		client: client,
		store:  store,
	}
}

// flushDiagnostics flushes collected diagnostics to the store if collection is enabled.
func (w *Wrapper) flushDiagnostics(ctx context.Context, respHeader func() http.Header) {
	diagCtx := From(ctx)
	if diagCtx == nil || !diagCtx.Collect {
		return
	}

	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil || len(tenantIDs) == 0 {
		level.Warn(w.logger).Log("msg", "failed to get tenant ID for diagnostics flush", "err", err)
		return
	}

	responseTimeMs := time.Since(diagCtx.startTime).Milliseconds()
	if err := w.store.Flush(ctx, tenantIDs[0], diagCtx.ID, responseTimeMs); err != nil {
		level.Warn(w.logger).Log("msg", "failed to flush diagnostics", "id", diagCtx.ID, "err", err)
	}

	// Set the diagnostics ID header in the response
	if respHeader != nil {
		respHeader().Set(idHeader, diagCtx.ID)
	}
}

func (w *Wrapper) ProfileTypes(ctx context.Context, req *connect.Request[querierv1.ProfileTypesRequest]) (*connect.Response[querierv1.ProfileTypesResponse], error) {
	resp, err := w.client.ProfileTypes(ctx, req)
	if err == nil {
		w.flushDiagnostics(ctx, resp.Header)
	}
	return resp, err
}

func (w *Wrapper) LabelValues(ctx context.Context, req *connect.Request[typesv1.LabelValuesRequest]) (*connect.Response[typesv1.LabelValuesResponse], error) {
	resp, err := w.client.LabelValues(ctx, req)
	if err == nil {
		w.flushDiagnostics(ctx, resp.Header)
	}
	return resp, err
}

func (w *Wrapper) LabelNames(ctx context.Context, req *connect.Request[typesv1.LabelNamesRequest]) (*connect.Response[typesv1.LabelNamesResponse], error) {
	resp, err := w.client.LabelNames(ctx, req)
	if err == nil {
		w.flushDiagnostics(ctx, resp.Header)
	}
	return resp, err
}

func (w *Wrapper) Series(ctx context.Context, req *connect.Request[querierv1.SeriesRequest]) (*connect.Response[querierv1.SeriesResponse], error) {
	resp, err := w.client.Series(ctx, req)
	if err == nil {
		w.flushDiagnostics(ctx, resp.Header)
	}
	return resp, err
}

func (w *Wrapper) SelectMergeStacktraces(ctx context.Context, req *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	resp, err := w.client.SelectMergeStacktraces(ctx, req)
	if err == nil {
		w.flushDiagnostics(ctx, resp.Header)
	}
	return resp, err
}

func (w *Wrapper) SelectMergeSpanProfile(ctx context.Context, req *connect.Request[querierv1.SelectMergeSpanProfileRequest]) (*connect.Response[querierv1.SelectMergeSpanProfileResponse], error) {
	resp, err := w.client.SelectMergeSpanProfile(ctx, req)
	if err == nil {
		w.flushDiagnostics(ctx, resp.Header)
	}
	return resp, err
}

func (w *Wrapper) SelectMergeProfile(ctx context.Context, req *connect.Request[querierv1.SelectMergeProfileRequest]) (*connect.Response[profilev1.Profile], error) {
	resp, err := w.client.SelectMergeProfile(ctx, req)
	if err == nil {
		w.flushDiagnostics(ctx, resp.Header)
	}
	return resp, err
}

func (w *Wrapper) SelectSeries(ctx context.Context, req *connect.Request[querierv1.SelectSeriesRequest]) (*connect.Response[querierv1.SelectSeriesResponse], error) {
	resp, err := w.client.SelectSeries(ctx, req)
	if err == nil {
		w.flushDiagnostics(ctx, resp.Header)
	}
	return resp, err
}

func (w *Wrapper) SelectHeatmap(ctx context.Context, req *connect.Request[querierv1.SelectHeatmapRequest]) (*connect.Response[querierv1.SelectHeatmapResponse], error) {
	resp, err := w.client.SelectHeatmap(ctx, req)
	if err == nil {
		w.flushDiagnostics(ctx, resp.Header)
	}
	return resp, err
}

func (w *Wrapper) Diff(ctx context.Context, req *connect.Request[querierv1.DiffRequest]) (*connect.Response[querierv1.DiffResponse], error) {
	resp, err := w.client.Diff(ctx, req)
	if err == nil {
		w.flushDiagnostics(ctx, resp.Header)
	}
	return resp, err
}

func (w *Wrapper) GetProfileStats(ctx context.Context, req *connect.Request[typesv1.GetProfileStatsRequest]) (*connect.Response[typesv1.GetProfileStatsResponse], error) {
	resp, err := w.client.GetProfileStats(ctx, req)
	if err == nil {
		w.flushDiagnostics(ctx, resp.Header)
	}
	return resp, err
}

func (w *Wrapper) AnalyzeQuery(ctx context.Context, req *connect.Request[querierv1.AnalyzeQueryRequest]) (*connect.Response[querierv1.AnalyzeQueryResponse], error) {
	resp, err := w.client.AnalyzeQuery(ctx, req)
	if err == nil {
		w.flushDiagnostics(ctx, resp.Header)
	}
	return resp, err
}

// Ensure Wrapper implements the interface
var _ querierv1connect.QuerierServiceClient = (*Wrapper)(nil)
