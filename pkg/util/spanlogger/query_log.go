package spanlogger

import (
	"context"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/dustin/go-humanize"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tracing"
	"github.com/prometheus/common/model"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

type LogSpanParametersWrapper struct {
	client querierv1connect.QuerierServiceClient
	logger log.Logger
}

func NewLogSpanParametersWrapper(client querierv1connect.QuerierServiceClient, logger log.Logger) *LogSpanParametersWrapper {
	return &LogSpanParametersWrapper{
		client: client,
		logger: logger,
	}
}

// logWithRequestMetadata returns a SpanLogger enriched with request metadata.
// Add new extractions here to make them available on all query log lines.
func (l LogSpanParametersWrapper) logWithRequestMetadata(ctx context.Context, req connect.AnyRequest) *SpanLogger {
	logger := l.logger
	if ua := req.Header().Get("User-Agent"); ua != "" {
		logger = log.With(logger, "user_agent", ua)
	}
	return FromContext(ctx, logger)
}

// logQuery emits a "query started" line, executes fn, then emits a "query
// finished" line with latency and (optionally) humanized bytes fetched.
// reqFields must be a flat key-value list of request-scoped fields that will
// appear on both lines.
func (l LogSpanParametersWrapper) logQuery(
	logger *SpanLogger,
	stats *QueryStats,
	reqFields []interface{},
	fn func() error,
) error {
	level.Info(logger).Log(append([]interface{}{"msg", "query started"}, reqFields...)...)

	t := time.Now()
	err := fn()
	latency := time.Since(t)

	finishFields := make([]interface{}, 0, len(reqFields)+8)
	finishFields = append(finishFields, "msg", "query finished")
	finishFields = append(finishFields, reqFields...)
	finishFields = append(finishFields, "latency", latency)
	if stats != nil {
		finishFields = append(finishFields,
			"fetched_object_bytes", humanize.Bytes(stats.ObjectStorageBytes),
			"fetched_metastore_bytes", humanize.Bytes(stats.MetastoreBytes),
			"estimated_object_bytes", humanize.Bytes(stats.EstimatedBytes),
		)
		if stats.ObjectStorageBytes > 0 {
			finishFields = append(finishFields,
				"estimation_ratio", fmt.Sprintf("%.2f", float64(stats.EstimatedBytes)/float64(stats.ObjectStorageBytes)),
			)
		}
	}
	level.Info(logger).Log(finishFields...)
	return err
}

func (l LogSpanParametersWrapper) ProfileTypes(ctx context.Context, c *connect.Request[querierv1.ProfileTypesRequest]) (*connect.Response[querierv1.ProfileTypesResponse], error) {
	spanName := "ProfileTypes"
	sp, ctx := tracing.StartSpanFromContext(ctx, spanName)
	defer sp.Finish()
	ctx, stats := ContextWithQueryStats(ctx)

	var resp *connect.Response[querierv1.ProfileTypesResponse]
	err := l.logQuery(l.logWithRequestMetadata(ctx, c), stats, []interface{}{
		"method", spanName,
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"query_window", model.Time(c.Msg.End).Sub(model.Time(c.Msg.Start)).String(),
	}, func() (err error) {
		resp, err = l.client.ProfileTypes(ctx, c)
		return err
	})
	return resp, err
}

func (l LogSpanParametersWrapper) LabelValues(ctx context.Context, c *connect.Request[typesv1.LabelValuesRequest]) (*connect.Response[typesv1.LabelValuesResponse], error) {
	spanName := "LabelValues"
	sp, ctx := tracing.StartSpanFromContext(ctx, spanName)
	defer sp.Finish()
	ctx, stats := ContextWithQueryStats(ctx)

	var resp *connect.Response[typesv1.LabelValuesResponse]
	err := l.logQuery(l.logWithRequestMetadata(ctx, c), stats, []interface{}{
		"method", spanName,
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"query_window", model.Time(c.Msg.End).Sub(model.Time(c.Msg.Start)).String(),
		"matchers", lazyJoin(c.Msg.Matchers),
		"name", c.Msg.Name,
	}, func() (err error) {
		resp, err = l.client.LabelValues(ctx, c)
		return err
	})
	return resp, err
}

func (l LogSpanParametersWrapper) LabelNames(ctx context.Context, c *connect.Request[typesv1.LabelNamesRequest]) (*connect.Response[typesv1.LabelNamesResponse], error) {
	spanName := "LabelNames"
	sp, ctx := tracing.StartSpanFromContext(ctx, spanName)
	defer sp.Finish()
	ctx, stats := ContextWithQueryStats(ctx)

	var resp *connect.Response[typesv1.LabelNamesResponse]
	err := l.logQuery(l.logWithRequestMetadata(ctx, c), stats, []interface{}{
		"method", spanName,
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"query_window", model.Time(c.Msg.End).Sub(model.Time(c.Msg.Start)).String(),
		"matchers", lazyJoin(c.Msg.Matchers),
	}, func() (err error) {
		resp, err = l.client.LabelNames(ctx, c)
		return err
	})
	return resp, err
}

func (l LogSpanParametersWrapper) Series(ctx context.Context, c *connect.Request[querierv1.SeriesRequest]) (*connect.Response[querierv1.SeriesResponse], error) {
	spanName := "Series"
	sp, ctx := tracing.StartSpanFromContext(ctx, spanName)
	defer sp.Finish()
	ctx, stats := ContextWithQueryStats(ctx)

	var resp *connect.Response[querierv1.SeriesResponse]
	err := l.logQuery(l.logWithRequestMetadata(ctx, c), stats, []interface{}{
		"method", spanName,
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"query_window", model.Time(c.Msg.End).Sub(model.Time(c.Msg.Start)).String(),
		"matchers", lazyJoin(c.Msg.Matchers),
		"label_names", lazyJoin(c.Msg.LabelNames),
	}, func() (err error) {
		resp, err = l.client.Series(ctx, c)
		return err
	})
	return resp, err
}

func (l LogSpanParametersWrapper) SelectMergeStacktraces(ctx context.Context, c *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	spanName := "SelectMergeStacktraces"
	sp, ctx := tracing.StartSpanFromContext(ctx, spanName)
	defer sp.Finish()
	ctx, stats := ContextWithQueryStats(ctx)

	var resp *connect.Response[querierv1.SelectMergeStacktracesResponse]
	err := l.logQuery(l.logWithRequestMetadata(ctx, c), stats, []interface{}{
		"method", spanName,
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"query_window", model.Time(c.Msg.End).Sub(model.Time(c.Msg.Start)).String(),
		"selector", c.Msg.LabelSelector,
		"profile_type", c.Msg.ProfileTypeID,
		"format", c.Msg.Format,
		"max_nodes", c.Msg.GetMaxNodes(),
		"profile_id_selector", lazyJoin(c.Msg.ProfileIdSelector),
	}, func() (err error) {
		resp, err = l.client.SelectMergeStacktraces(ctx, c)
		return err
	})
	return resp, err
}

func (l LogSpanParametersWrapper) SelectMergeSpanProfile(ctx context.Context, c *connect.Request[querierv1.SelectMergeSpanProfileRequest]) (*connect.Response[querierv1.SelectMergeSpanProfileResponse], error) {
	spanName := "SelectMergeSpanProfile"
	sp, ctx := tracing.StartSpanFromContext(ctx, spanName)
	defer sp.Finish()
	ctx, stats := ContextWithQueryStats(ctx)

	var resp *connect.Response[querierv1.SelectMergeSpanProfileResponse]
	err := l.logQuery(l.logWithRequestMetadata(ctx, c), stats, []interface{}{
		"method", spanName,
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"query_window", model.Time(c.Msg.End).Sub(model.Time(c.Msg.Start)).String(),
		"selector", c.Msg.LabelSelector,
		"profile_type", c.Msg.ProfileTypeID,
		"format", c.Msg.Format,
		"max_nodes", c.Msg.GetMaxNodes(),
	}, func() (err error) {
		resp, err = l.client.SelectMergeSpanProfile(ctx, c)
		return err
	})
	return resp, err
}

func (l LogSpanParametersWrapper) SelectMergeProfile(ctx context.Context, c *connect.Request[querierv1.SelectMergeProfileRequest]) (*connect.Response[profilev1.Profile], error) {
	spanName := "SelectMergeProfile"
	sp, ctx := tracing.StartSpanFromContext(ctx, spanName)
	defer sp.Finish()
	ctx, stats := ContextWithQueryStats(ctx)

	var resp *connect.Response[profilev1.Profile]
	err := l.logQuery(l.logWithRequestMetadata(ctx, c), stats, []interface{}{
		"method", spanName,
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"query_window", model.Time(c.Msg.End).Sub(model.Time(c.Msg.Start)).String(),
		"selector", c.Msg.LabelSelector,
		"max_nodes", c.Msg.GetMaxNodes(),
		"profile_type", c.Msg.ProfileTypeID,
		"stacktrace_selector", c.Msg.GetStackTraceSelector(),
		"profile_id_selector", lazyJoin(c.Msg.ProfileIdSelector),
	}, func() (err error) {
		resp, err = l.client.SelectMergeProfile(ctx, c)
		return err
	})
	return resp, err
}

func (l LogSpanParametersWrapper) SelectSeries(ctx context.Context, c *connect.Request[querierv1.SelectSeriesRequest]) (*connect.Response[querierv1.SelectSeriesResponse], error) {
	spanName := "SelectSeries"
	sp, ctx := tracing.StartSpanFromContext(ctx, spanName)
	defer sp.Finish()
	ctx, stats := ContextWithQueryStats(ctx)

	var resp *connect.Response[querierv1.SelectSeriesResponse]
	err := l.logQuery(l.logWithRequestMetadata(ctx, c), stats, []interface{}{
		"method", spanName,
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"query_window", model.Time(c.Msg.End).Sub(model.Time(c.Msg.Start)).String(),
		"selector", c.Msg.LabelSelector,
		"profile_type", c.Msg.ProfileTypeID,
		"stacktrace_selector", c.Msg.GetStackTraceSelector(),
		"step", c.Msg.Step,
		"by", lazyJoin(c.Msg.GroupBy),
		"aggregation", c.Msg.GetAggregation().String(),
		"limit", c.Msg.Limit,
		"exemplar_type", c.Msg.ExemplarType,
	}, func() (err error) {
		resp, err = l.client.SelectSeries(ctx, c)
		return err
	})
	return resp, err
}

func (l LogSpanParametersWrapper) SelectHeatmap(ctx context.Context, c *connect.Request[querierv1.SelectHeatmapRequest]) (*connect.Response[querierv1.SelectHeatmapResponse], error) {
	spanName := "SelectHeatmap"
	sp, ctx := tracing.StartSpanFromContext(ctx, spanName)
	defer sp.Finish()
	ctx, stats := ContextWithQueryStats(ctx)

	var resp *connect.Response[querierv1.SelectHeatmapResponse]
	err := l.logQuery(l.logWithRequestMetadata(ctx, c), stats, []interface{}{
		"method", spanName,
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"query_window", model.Time(c.Msg.End).Sub(model.Time(c.Msg.Start)).String(),
		"selector", c.Msg.LabelSelector,
		"profile_type", c.Msg.ProfileTypeID,
		"step", c.Msg.Step,
		"by", lazyJoin(c.Msg.GroupBy),
		"query_type", c.Msg.QueryType,
		"exemplar_type", c.Msg.ExemplarType,
		"limit", c.Msg.Limit,
	}, func() (err error) {
		resp, err = l.client.SelectHeatmap(ctx, c)
		return err
	})
	return resp, err
}

func (l LogSpanParametersWrapper) Diff(ctx context.Context, c *connect.Request[querierv1.DiffRequest]) (*connect.Response[querierv1.DiffResponse], error) {
	spanName := "Diff"
	sp, ctx := tracing.StartSpanFromContext(ctx, spanName)
	defer sp.Finish()
	ctx, stats := ContextWithQueryStats(ctx)

	left := &querierv1.SelectMergeStacktracesRequest{}
	if c.Msg.Left != nil {
		left = c.Msg.Left
	}
	right := &querierv1.SelectMergeStacktracesRequest{}
	if c.Msg.Right != nil {
		right = c.Msg.Right
	}

	var resp *connect.Response[querierv1.DiffResponse]
	err := l.logQuery(l.logWithRequestMetadata(ctx, c), stats, []interface{}{
		"method", spanName,
		"left_start", model.Time(left.Start).Time().String(),
		"left_end", model.Time(left.End).Time().String(),
		"left_query_window", model.Time(left.End).Sub(model.Time(left.Start)).String(),
		"left_selector", left.LabelSelector,
		"left_profile_type", left.ProfileTypeID,
		"left_format", left.Format,
		"left_max_nodes", left.GetMaxNodes(),
		"right_start", model.Time(right.Start).Time().String(),
		"right_end", model.Time(right.End).Time().String(),
		"right_query_window", model.Time(right.End).Sub(model.Time(right.Start)).String(),
		"right_selector", right.LabelSelector,
		"right_profile_type", right.ProfileTypeID,
		"right_format", right.Format,
		"right_max_nodes", right.GetMaxNodes(),
	}, func() (err error) {
		resp, err = l.client.Diff(ctx, c)
		return err
	})
	return resp, err
}

func (l LogSpanParametersWrapper) GetProfileStats(ctx context.Context, c *connect.Request[typesv1.GetProfileStatsRequest]) (*connect.Response[typesv1.GetProfileStatsResponse], error) {
	sp, ctx := tracing.StartSpanFromContext(ctx, "GetProfileStats")
	defer sp.Finish()

	return l.client.GetProfileStats(ctx, c)
}

func (l LogSpanParametersWrapper) AnalyzeQuery(ctx context.Context, c *connect.Request[querierv1.AnalyzeQueryRequest]) (*connect.Response[querierv1.AnalyzeQueryResponse], error) {
	spanName := "AnalyzeQuery"
	sp, ctx := tracing.StartSpanFromContext(ctx, spanName)
	defer sp.Finish()
	ctx, stats := ContextWithQueryStats(ctx)

	var resp *connect.Response[querierv1.AnalyzeQueryResponse]
	err := l.logQuery(l.logWithRequestMetadata(ctx, c), stats, []interface{}{
		"method", spanName,
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"query_window", model.Time(c.Msg.End).Sub(model.Time(c.Msg.Start)).String(),
		"query", c.Msg.Query,
	}, func() (err error) {
		resp, err = l.client.AnalyzeQuery(ctx, c)
		return err
	})
	return resp, err
}

type LazyJoin struct {
	strs []string
	sep  string
}

func (l *LazyJoin) String() string {
	return strings.Join(l.strs, l.sep)
}

func lazyJoin(strs []string) *LazyJoin {
	return &LazyJoin{strs: strs, sep: ","}
}
