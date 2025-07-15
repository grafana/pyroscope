package spanlogger

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/opentracing/opentracing-go"
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

func (l LogSpanParametersWrapper) ProfileTypes(ctx context.Context, c *connect.Request[querierv1.ProfileTypesRequest]) (*connect.Response[querierv1.ProfileTypesResponse], error) {
	spanName := "ProfileTypes"
	sp, ctx := opentracing.StartSpanFromContext(ctx, spanName)
	level.Info(FromContext(ctx, l.logger)).Log(
		"method", spanName,
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"query_window", model.Time(c.Msg.End).Sub(model.Time(c.Msg.Start)).String(),
	)
	defer sp.Finish()

	return l.client.ProfileTypes(ctx, c)
}

func (l LogSpanParametersWrapper) LabelValues(ctx context.Context, c *connect.Request[typesv1.LabelValuesRequest]) (*connect.Response[typesv1.LabelValuesResponse], error) {
	spanName := "LabelValues"
	sp, ctx := opentracing.StartSpanFromContext(ctx, spanName)
	level.Info(FromContext(ctx, l.logger)).Log(
		"method", spanName,
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"query_window", model.Time(c.Msg.End).Sub(model.Time(c.Msg.Start)).String(),
		"matchers", lazyJoin(c.Msg.Matchers),
		"name", c.Msg.Name,
	)
	defer sp.Finish()

	return l.client.LabelValues(ctx, c)
}

func (l LogSpanParametersWrapper) LabelNames(ctx context.Context, c *connect.Request[typesv1.LabelNamesRequest]) (*connect.Response[typesv1.LabelNamesResponse], error) {
	spanName := "LabelNames"
	sp, ctx := opentracing.StartSpanFromContext(ctx, spanName)
	level.Info(FromContext(ctx, l.logger)).Log(
		"method", spanName,
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"query_window", model.Time(c.Msg.End).Sub(model.Time(c.Msg.Start)).String(),
		"matchers", lazyJoin(c.Msg.Matchers),
	)
	defer sp.Finish()

	return l.client.LabelNames(ctx, c)
}

func (l LogSpanParametersWrapper) Series(ctx context.Context, c *connect.Request[querierv1.SeriesRequest]) (*connect.Response[querierv1.SeriesResponse], error) {
	spanName := "Series"
	sp, ctx := opentracing.StartSpanFromContext(ctx, spanName)
	level.Info(FromContext(ctx, l.logger)).Log(
		"method", spanName,
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"query_window", model.Time(c.Msg.End).Sub(model.Time(c.Msg.Start)).String(),
		"matchers", lazyJoin(c.Msg.Matchers),
		"label_names", lazyJoin(c.Msg.LabelNames),
	)
	defer sp.Finish()

	return l.client.Series(ctx, c)
}

func (l LogSpanParametersWrapper) SelectMergeStacktraces(ctx context.Context, c *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	spanName := "SelectMergeStacktraces"
	sp, ctx := opentracing.StartSpanFromContext(ctx, spanName)
	level.Info(FromContext(ctx, l.logger)).Log(
		"method", spanName,
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"query_window", model.Time(c.Msg.End).Sub(model.Time(c.Msg.Start)).String(),
		"selector", c.Msg.LabelSelector,
		"profile_type", c.Msg.ProfileTypeID,
		"format", c.Msg.Format,
		"max_nodes", c.Msg.GetMaxNodes(),
	)
	defer sp.Finish()

	return l.client.SelectMergeStacktraces(ctx, c)
}

func (l LogSpanParametersWrapper) SelectMergeSpanProfile(ctx context.Context, c *connect.Request[querierv1.SelectMergeSpanProfileRequest]) (*connect.Response[querierv1.SelectMergeSpanProfileResponse], error) {
	spanName := "SelectMergeSpanProfile"
	sp, ctx := opentracing.StartSpanFromContext(ctx, spanName)
	level.Info(FromContext(ctx, l.logger)).Log(
		"method", spanName,
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"query_window", model.Time(c.Msg.End).Sub(model.Time(c.Msg.Start)).String(),
		"selector", c.Msg.LabelSelector,
		"profile_type", c.Msg.ProfileTypeID,
		"format", c.Msg.Format,
		"max_nodes", c.Msg.GetMaxNodes(),
	)
	defer sp.Finish()

	return l.client.SelectMergeSpanProfile(ctx, c)
}

func (l LogSpanParametersWrapper) SelectMergeProfile(ctx context.Context, c *connect.Request[querierv1.SelectMergeProfileRequest]) (*connect.Response[profilev1.Profile], error) {
	spanName := "SelectMergeProfile"
	sp, ctx := opentracing.StartSpanFromContext(ctx, spanName)
	level.Info(FromContext(ctx, l.logger)).Log(
		"method", spanName,
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"query_window", model.Time(c.Msg.End).Sub(model.Time(c.Msg.Start)).String(),
		"selector", c.Msg.LabelSelector,
		"max_nodes", c.Msg.GetMaxNodes(),
		"profile_type", c.Msg.ProfileTypeID,
		"stacktrace_selector", c.Msg.StackTraceSelector,
	)
	defer sp.Finish()

	return l.client.SelectMergeProfile(ctx, c)
}

func (l LogSpanParametersWrapper) SelectSeries(ctx context.Context, c *connect.Request[querierv1.SelectSeriesRequest]) (*connect.Response[querierv1.SelectSeriesResponse], error) {
	spanName := "SelectSeries"
	sp, ctx := opentracing.StartSpanFromContext(ctx, spanName)
	level.Info(FromContext(ctx, l.logger)).Log(
		"method", spanName,
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"query_window", model.Time(c.Msg.End).Sub(model.Time(c.Msg.Start)).String(),
		"selector", c.Msg.LabelSelector,
		"profile_type", c.Msg.ProfileTypeID,
		"stacktrace_selector", c.Msg.StackTraceSelector,
		"step", c.Msg.Step,
		"by", lazyJoin(c.Msg.GroupBy),
		"aggregation", c.Msg.Aggregation,
		"limit", c.Msg.Limit,
	)
	defer sp.Finish()

	return l.client.SelectSeries(ctx, c)
}

func (l LogSpanParametersWrapper) Diff(ctx context.Context, c *connect.Request[querierv1.DiffRequest]) (*connect.Response[querierv1.DiffResponse], error) {
	spanName := "Diff"
	sp, ctx := opentracing.StartSpanFromContext(ctx, spanName)
	level.Info(FromContext(ctx, l.logger)).Log(
		"method", spanName,
		"left_start", model.Time(c.Msg.Left.Start).Time().String(),
		"left_end", model.Time(c.Msg.Left.End).Time().String(),
		"left_query_window", model.Time(c.Msg.Left.End).Sub(model.Time(c.Msg.Left.Start)).String(),
		"left_selector", c.Msg.Left.LabelSelector,
		"left_profile_type", c.Msg.Left.ProfileTypeID,
		"left_format", c.Msg.Left.Format,
		"left_max_nodes", c.Msg.Left.GetMaxNodes(),
		"right_start", model.Time(c.Msg.Right.Start).Time().String(),
		"right_end", model.Time(c.Msg.Right.End).Time().String(),
		"right_query_window", model.Time(c.Msg.Right.End).Sub(model.Time(c.Msg.Right.Start)).String(),
		"right_selector", c.Msg.Right.LabelSelector,
		"right_profile_type", c.Msg.Right.ProfileTypeID,
		"right_format", c.Msg.Right.Format,
		"right_max_nodes", c.Msg.Right.GetMaxNodes(),
	)
	defer sp.Finish()

	return l.client.Diff(ctx, c)
}

func (l LogSpanParametersWrapper) GetProfileStats(ctx context.Context, c *connect.Request[typesv1.GetProfileStatsRequest]) (*connect.Response[typesv1.GetProfileStatsResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "GetProfileStats")
	defer sp.Finish()

	return l.client.GetProfileStats(ctx, c)
}

func (l LogSpanParametersWrapper) AnalyzeQuery(ctx context.Context, c *connect.Request[querierv1.AnalyzeQueryRequest]) (*connect.Response[querierv1.AnalyzeQueryResponse], error) {
	spanName := "AnalyzeQuery"
	sp, ctx := opentracing.StartSpanFromContext(ctx, spanName)
	level.Info(FromContext(ctx, l.logger)).Log(
		"method", spanName,
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"query_window", model.Time(c.Msg.End).Sub(model.Time(c.Msg.Start)).String(),
		"query", c.Msg.Query,
	)
	defer sp.Finish()

	return l.client.AnalyzeQuery(ctx, c)
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
