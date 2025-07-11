package spanlogger

import (
	"context"

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
	sp, ctx := opentracing.StartSpanFromContext(ctx, "ProfileTypes")
	level.Info(FromContext(ctx, l.logger)).Log(
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
	)
	defer sp.Finish()

	return l.client.ProfileTypes(ctx, c)
}

func (l LogSpanParametersWrapper) LabelValues(ctx context.Context, c *connect.Request[typesv1.LabelValuesRequest]) (*connect.Response[typesv1.LabelValuesResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "LabelValues")
	level.Info(FromContext(ctx, l.logger)).Log(
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"matchers", c.Msg.Matchers,
		"name", c.Msg.Name,
	)
	defer sp.Finish()

	return l.client.LabelValues(ctx, c)
}

func (l LogSpanParametersWrapper) LabelNames(ctx context.Context, c *connect.Request[typesv1.LabelNamesRequest]) (*connect.Response[typesv1.LabelNamesResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "LabelNames")
	level.Info(FromContext(ctx, l.logger)).Log(
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"matchers", c.Msg.Matchers,
	)
	defer sp.Finish()

	return l.client.LabelNames(ctx, c)
}

func (l LogSpanParametersWrapper) Series(ctx context.Context, c *connect.Request[querierv1.SeriesRequest]) (*connect.Response[querierv1.SeriesResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "Series")
	level.Info(FromContext(ctx, l.logger)).Log(
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"matchers", c.Msg.Matchers,
		"label_names", c.Msg.LabelNames,
	)
	defer sp.Finish()

	return l.client.Series(ctx, c)
}

func (l LogSpanParametersWrapper) SelectMergeStacktraces(ctx context.Context, c *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectMergeStacktraces")
	level.Info(FromContext(ctx, l.logger)).Log(
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"selector", c.Msg.LabelSelector,
		"profile_type", c.Msg.ProfileTypeID,
		"format", c.Msg.Format,
		"max_nodes", c.Msg.GetMaxNodes(),
	)
	defer sp.Finish()

	return l.client.SelectMergeStacktraces(ctx, c)
}

func (l LogSpanParametersWrapper) SelectMergeSpanProfile(ctx context.Context, c *connect.Request[querierv1.SelectMergeSpanProfileRequest]) (*connect.Response[querierv1.SelectMergeSpanProfileResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectMergeSpanProfile")
	level.Info(FromContext(ctx, l.logger)).Log(
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"selector", c.Msg.LabelSelector,
		"profile_type", c.Msg.ProfileTypeID,
		"format", c.Msg.Format,
		"max_nodes", c.Msg.GetMaxNodes(),
	)
	defer sp.Finish()

	return l.client.SelectMergeSpanProfile(ctx, c)
}

func (l LogSpanParametersWrapper) SelectMergeProfile(ctx context.Context, c *connect.Request[querierv1.SelectMergeProfileRequest]) (*connect.Response[profilev1.Profile], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectMergeProfile")
	level.Info(FromContext(ctx, l.logger)).Log(
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"selector", c.Msg.LabelSelector,
		"max_nodes", c.Msg.GetMaxNodes(),
		"profile_type", c.Msg.ProfileTypeID,
		"stacktrace_selector", c.Msg.StackTraceSelector,
	)
	defer sp.Finish()

	return l.client.SelectMergeProfile(ctx, c)
}

func (l LogSpanParametersWrapper) SelectSeries(ctx context.Context, c *connect.Request[querierv1.SelectSeriesRequest]) (*connect.Response[querierv1.SelectSeriesResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectSeries")
	level.Info(FromContext(ctx, l.logger)).Log(
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"selector", c.Msg.LabelSelector,
		"profile_type", c.Msg.ProfileTypeID,
		"stacktrace_selector", c.Msg.StackTraceSelector,
		"step", c.Msg.Step,
		"by", c.Msg.GroupBy,
		"aggregation", c.Msg.Aggregation,
		"limit", c.Msg.Limit,
	)
	defer sp.Finish()

	return l.client.SelectSeries(ctx, c)
}

func (l LogSpanParametersWrapper) Diff(ctx context.Context, c *connect.Request[querierv1.DiffRequest]) (*connect.Response[querierv1.DiffResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "Diff")
	level.Info(FromContext(ctx, l.logger)).Log(
		"left_start", model.Time(c.Msg.Left.Start).Time().String(),
		"left_end", model.Time(c.Msg.Left.End).Time().String(),
		"left_selector", c.Msg.Left.LabelSelector,
		"left_profile_type", c.Msg.Left.ProfileTypeID,
		"left_format", c.Msg.Left.Format,
		"left_max_nodes", c.Msg.Left.GetMaxNodes(),
		"right_start", model.Time(c.Msg.Right.Start).Time().String(),
		"right_end", model.Time(c.Msg.Right.End).Time().String(),
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
	sp, ctx := opentracing.StartSpanFromContext(ctx, "AnalyzeQuery")
	level.Info(FromContext(ctx, l.logger)).Log(
		"start", model.Time(c.Msg.Start).Time().String(),
		"end", model.Time(c.Msg.End).Time().String(),
		"query", c.Msg.Query,
	)
	defer sp.Finish()

	return l.client.AnalyzeQuery(ctx, c)
}
