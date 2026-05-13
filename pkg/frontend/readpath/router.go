package readpath

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tenant"
	"github.com/prometheus/common/model"
	"golang.org/x/sync/errgroup"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
)

type Overrides interface {
	ReadPathOverrides(tenantID string) Config
}

// unaryQuerierSvc is satisfied by both QuerierServiceClient and
// QuerierServiceHandler — all unary methods share identical signatures.
// Using this avoids the naming conflict on server-streaming methods between
// the two generated interfaces.
type unaryQuerierSvc interface {
	ProfileTypes(context.Context, *connect.Request[querierv1.ProfileTypesRequest]) (*connect.Response[querierv1.ProfileTypesResponse], error)
	LabelValues(context.Context, *connect.Request[typesv1.LabelValuesRequest]) (*connect.Response[typesv1.LabelValuesResponse], error)
	LabelNames(context.Context, *connect.Request[typesv1.LabelNamesRequest]) (*connect.Response[typesv1.LabelNamesResponse], error)
	Series(context.Context, *connect.Request[querierv1.SeriesRequest]) (*connect.Response[querierv1.SeriesResponse], error)
	SelectMergeStacktraces(context.Context, *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error)
	SelectMergeSpanProfile(context.Context, *connect.Request[querierv1.SelectMergeSpanProfileRequest]) (*connect.Response[querierv1.SelectMergeSpanProfileResponse], error)
	SelectMergeProfile(context.Context, *connect.Request[querierv1.SelectMergeProfileRequest]) (*connect.Response[googlev1.Profile], error)
	SelectSeries(context.Context, *connect.Request[querierv1.SelectSeriesRequest]) (*connect.Response[querierv1.SelectSeriesResponse], error)
	SelectHeatmap(context.Context, *connect.Request[querierv1.SelectHeatmapRequest]) (*connect.Response[querierv1.SelectHeatmapResponse], error)
	Diff(context.Context, *connect.Request[querierv1.DiffRequest]) (*connect.Response[querierv1.DiffResponse], error)
	GetProfileStats(context.Context, *connect.Request[typesv1.GetProfileStatsRequest]) (*connect.Response[typesv1.GetProfileStatsResponse], error)
	AnalyzeQuery(context.Context, *connect.Request[querierv1.AnalyzeQueryRequest]) (*connect.Response[querierv1.AnalyzeQueryResponse], error)
}

// SplitTimeResolver resolves the split time for a tenant in "auto" mode.
// It returns the oldest profile time known for the tenant in the v2 storage.
type SplitTimeResolver interface {
	OldestProfileTime(ctx context.Context, tenantID string) (time.Time, error)
}

// Router is a proxy that routes queries to the query frontend.
//
// If the query backend is enabled, it routes queries to the new
// query frontend, otherwise it routes queries to the old query
// frontend.
//
// If the query targets a time range that spans the enablement of
// the new query backend, it splits the query into two parts and
// sends them to the old and new query frontends.
type Router struct {
	logger    log.Logger
	overrides Overrides
	resolver  SplitTimeResolver

	oldFrontend unaryQuerierSvc
	newFrontend unaryQuerierSvc
}

func NewRouter(
	logger log.Logger,
	overrides Overrides,
	resolver SplitTimeResolver,
	oldFrontend unaryQuerierSvc,
	newFrontend unaryQuerierSvc,
) *Router {
	return &Router{
		logger:      logger,
		overrides:   overrides,
		resolver:    resolver,
		oldFrontend: oldFrontend,
		newFrontend: newFrontend,
	}
}

// Query routes a query to the appropriate query frontend.
// Before the call to the frontend is made, the requests
// are sanitized: any of the arguments can be nil, but not
// both. If the query was split, the responses are aggregated.
func Query[Req, Resp any](
	ctx context.Context,
	router *Router,
	req *connect.Request[Req],
	sanitize func(a, b *Req),
	aggregate func(a, b *Resp) (*Resp, error),
) (*connect.Response[Resp], error) {
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if len(tenantIDs) != 1 {
		level.Warn(router.logger).Log("msg", "ignoring inter-tenant query overrides", "tenants", tenantIDs)
	}
	tenantID := tenantIDs[0]

	// Verbose but explicit. Note that limits, error handling, etc.,
	// are delegated to the callee.
	overrides := router.overrides.ReadPathOverrides(tenantID)
	if !overrides.EnableQueryBackend {
		sanitize(req.Msg, nil)
		return query[Req, Resp](ctx, router.oldFrontend, req)
	}

	splitTime, err := overrides.EnableQueryBackendFrom.SplitTime(func() (time.Time, error) {
		return router.resolver.OldestProfileTime(ctx, tenantID)
	})
	if err != nil {
		level.Warn(router.logger).Log("msg", "failed to resolve split time, falling back to old frontend", "err", err)
		sanitize(req.Msg, nil)
		return query[Req, Resp](ctx, router.oldFrontend, req)
	}
	level.Debug(router.logger).Log("msg", "resolved split time for query backend routing", "split_time", splitTime.UTC().Format(time.RFC3339))

	// Note: the old read path includes both start and end: [start, end].
	// The new read path does not include end: [start, end).
	split := model.TimeFromUnixNano(splitTime.UnixNano())
	queryRange := phlaremodel.GetSafeTimeRange(time.Now(), req.Msg)
	if split.After(queryRange.End) {
		sanitize(req.Msg, nil)
		return query[Req, Resp](ctx, router.oldFrontend, req)
	}
	if split.Before(queryRange.Start) {
		sanitize(nil, req.Msg)
		return query[Req, Resp](ctx, router.newFrontend, req)
	}

	// We need to send requests both to the old and new read paths:
	// [start, split](split, end), which translates to
	// [start, split-1][split, end).
	c, ok := (any)(req.Msg).(interface{ CloneVT() *Req })
	if !ok {
		return nil, connect.NewError(connect.CodeUnimplemented, nil)
	}
	cloned := c.CloneVT()
	phlaremodel.SetTimeRange(req.Msg, queryRange.Start, split-1)
	phlaremodel.SetTimeRange(cloned, split, queryRange.End)
	sanitize(req.Msg, cloned)

	var a, b *connect.Response[Resp]
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		a, err = query[Req, Resp](ctx, router.oldFrontend, req)
		return err
	})
	g.Go(func() error {
		var err error
		b, err = query[Req, Resp](ctx, router.newFrontend, connect.NewRequest(cloned))
		return err
	})
	if err = g.Wait(); err != nil {
		return nil, err
	}

	resp, err := aggregate(a.Msg, b.Msg)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(resp), nil
}

func query[Req, Resp any](
	ctx context.Context,
	svc unaryQuerierSvc,
	req *connect.Request[Req],
) (*connect.Response[Resp], error) {
	var resp any
	var err error

	switch r := (any)(req).(type) {
	case *connect.Request[querierv1.ProfileTypesRequest]:
		resp, err = svc.ProfileTypes(ctx, r)
	case *connect.Request[typesv1.GetProfileStatsRequest]:
		resp, err = svc.GetProfileStats(ctx, r)
	case *connect.Request[querierv1.AnalyzeQueryRequest]:
		resp, err = svc.AnalyzeQuery(ctx, r)

	case *connect.Request[typesv1.LabelNamesRequest]:
		resp, err = svc.LabelNames(ctx, r)
	case *connect.Request[typesv1.LabelValuesRequest]:
		resp, err = svc.LabelValues(ctx, r)
	case *connect.Request[querierv1.SeriesRequest]:
		resp, err = svc.Series(ctx, r)

	case *connect.Request[querierv1.SelectMergeStacktracesRequest]:
		resp, err = svc.SelectMergeStacktraces(ctx, r)
	case *connect.Request[querierv1.SelectMergeSpanProfileRequest]:
		resp, err = svc.SelectMergeSpanProfile(ctx, r)
	case *connect.Request[querierv1.SelectMergeProfileRequest]:
		resp, err = svc.SelectMergeProfile(ctx, r)
	case *connect.Request[querierv1.SelectSeriesRequest]:
		resp, err = svc.SelectSeries(ctx, r)
	case *connect.Request[querierv1.SelectHeatmapRequest]:
		resp, err = svc.SelectHeatmap(ctx, r)
	case *connect.Request[querierv1.DiffRequest]:
		resp, err = svc.Diff(ctx, r)

	default:
		return nil, connect.NewError(connect.CodeUnimplemented, nil)
	}

	if err != nil || resp == nil {
		return nil, err
	}

	return resp.(*connect.Response[Resp]), nil
}
