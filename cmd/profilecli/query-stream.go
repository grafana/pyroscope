package main

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/dustin/go-humanize"
	"github.com/go-kit/log/level"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

type queryStreamFlamegraphParams struct {
	*queryProfileParams
}

func addQueryStreamFlamegraphParams(cmd commander) *queryStreamFlamegraphParams {
	params := new(queryStreamFlamegraphParams)
	params.queryProfileParams = addQueryProfileParams(cmd)
	return params
}

type queryStreamSeriesParams struct {
	*queryParams
	ProfileType string
	GroupBy     []string
}

func addQueryStreamSeriesParams(cmd commander) *queryStreamSeriesParams {
	params := new(queryStreamSeriesParams)
	params.queryParams = addQueryParams(cmd)
	cmd.Flag("profile-type", "Profile type to query.").Default("process_cpu:cpu:nanoseconds:cpu:nanoseconds").StringVar(&params.ProfileType)
	cmd.Flag("group-by", "Group series by label name(s). Can be specified multiple times.").StringsVar(&params.GroupBy)
	return params
}

func queryStreamFlamegraph(ctx context.Context, params *queryStreamFlamegraphParams) error {
	from, to, err := params.parseFromTo()
	if err != nil {
		return err
	}

	level.Info(logger).Log("msg", "streaming flamegraph", "url", params.URL, "from", from, "to", to, "query", params.Query, "type", params.ProfileType)

	req := &querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID: params.ProfileType,
		Start:         from.UnixMilli(),
		End:           to.UnixMilli(),
		LabelSelector: params.Query,
		Format:        querierv1.ProfileFormat_PROFILE_FORMAT_FLAMEGRAPH,
	}
	if params.MaxNodes > 0 {
		req.MaxNodes = &params.MaxNodes
	}

	start := time.Now()
	qc := params.phlareClient.streamQueryClient()
	stream, err := qc.SelectMergeStacktracesStream(ctx, connect.NewRequest(req))
	if err != nil {
		return fmt.Errorf("open stream: %w", err)
	}
	defer stream.Close()

	var msgCount int
	for stream.Receive() {
		msgCount++
		elapsed := time.Since(start).Truncate(time.Millisecond)
		switch k := stream.Msg().Kind.(type) {
		case *querierv1.FlamegraphStreamEvent_Progress:
			p := k.Progress
			nodes := 0
			if p.Flamegraph != nil {
				nodes = len(p.Flamegraph.Names)
			}
			eta := ""
			if p.EtaUnixMs > 0 {
				eta = fmt.Sprintf(" eta=%s", time.UnixMilli(p.EtaUnixMs).Format(time.TimeOnly))
			}
			fmt.Fprintf(consoleOutput, "[%s] progress  bytes_total=%s bytes_done=%s%s nodes=%d\n",
				elapsed,
				humanize.Bytes(p.BytesTotalEstimate),
				humanize.Bytes(p.BytesDone),
				eta,
				nodes)
		case *querierv1.FlamegraphStreamEvent_Flamegraph:
			fg := k.Flamegraph
			nodes, total := 0, int64(0)
			if fg != nil {
				nodes = len(fg.Names)
				total = fg.Total
			}
			fmt.Fprintf(consoleOutput, "[%s] result    nodes=%d total=%d\n", elapsed, nodes, total)
		}
	}
	if err := stream.Err(); err != nil {
		return fmt.Errorf("stream: %w", err)
	}

	level.Info(logger).Log("msg", "stream complete", "messages", msgCount, "elapsed", time.Since(start).Truncate(time.Millisecond))
	return nil
}

func queryStreamSeries(ctx context.Context, params *queryStreamSeriesParams) error {
	from, to, err := params.parseFromTo()
	if err != nil {
		return err
	}

	level.Info(logger).Log("msg", "streaming series", "url", params.URL, "from", from, "to", to, "query", params.Query, "type", params.ProfileType)

	stepSeconds := to.Sub(from).Seconds()
	req := &querierv1.SelectSeriesRequest{
		ProfileTypeID: params.ProfileType,
		Start:         from.UnixMilli(),
		End:           to.UnixMilli(),
		LabelSelector: params.Query,
		GroupBy:       params.GroupBy,
		Step:          stepSeconds,
	}

	start := time.Now()
	qc := params.phlareClient.streamQueryClient()
	stream, err := qc.SelectSeriesStream(ctx, connect.NewRequest(req))
	if err != nil {
		return fmt.Errorf("open stream: %w", err)
	}
	defer stream.Close()

	var msgCount int
	for stream.Receive() {
		msgCount++
		elapsed := time.Since(start).Truncate(time.Millisecond)
		switch k := stream.Msg().Kind.(type) {
		case *querierv1.SeriesStreamEvent_Progress:
			p := k.Progress
			eta := ""
			if p.EtaUnixMs > 0 {
				eta = fmt.Sprintf(" eta=%s", time.UnixMilli(p.EtaUnixMs).Format(time.TimeOnly))
			}
			fmt.Fprintf(consoleOutput, "[%s] progress  bytes_total=%s bytes_done=%s%s series=%d\n",
				elapsed,
				humanize.Bytes(p.BytesTotalEstimate),
				humanize.Bytes(p.BytesDone),
				eta,
				len(p.Series))
		case *querierv1.SeriesStreamEvent_Result:
			fmt.Fprintf(consoleOutput, "[%s] result    series=%d\n", elapsed, len(k.Result.GetSeries()))
		}
	}
	if err := stream.Err(); err != nil {
		return fmt.Errorf("stream: %w", err)
	}

	level.Info(logger).Log("msg", "stream complete", "messages", msgCount, "elapsed", time.Since(start).Truncate(time.Millisecond))
	return nil
}
