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

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

type Overrides interface {
	ReadPathOverrides(tenantID string) Config
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

	oldFrontend querierv1connect.QuerierServiceClient
	newFrontend querierv1connect.QuerierServiceClient
}

func NewRouter(
	logger log.Logger,
	overrides Overrides,
	oldFrontend querierv1connect.QuerierServiceClient,
	newFrontend querierv1connect.QuerierServiceClient,
) *Router {
	return &Router{
		logger:      logger,
		overrides:   overrides,
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
	// Note: the old read path includes both start and end: [start, end].
	// The new read path does not include end: [start, end).
	split := model.TimeFromUnixNano(overrides.EnableQueryBackendFrom.UnixNano())
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
	svc querierv1connect.QuerierServiceClient,
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
