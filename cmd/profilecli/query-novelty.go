package main

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/pkg/pprof/novelty"
)

type queryNoveltyParams struct {
	*queryParams
	TopN        uint64        // TOPk stacktraces
	StepSize    time.Duration // Step size for the novelty query
	ProfileType string
}

func addQueryNoveltyParams(queryCmd commander) *queryNoveltyParams {
	params := new(queryNoveltyParams)
	params.queryParams = addQueryParams(queryCmd)
	queryCmd.Flag("profile-type", "Profile type to query.").Default("process_cpu:cpu:nanoseconds:cpu:nanoseconds").StringVar(&params.ProfileType)
	queryCmd.Flag("step-size", "Show the top N stacktraces").Default("15s").DurationVar(&params.StepSize)
	queryCmd.Flag("top-n", "Show the top N stacktraces").Default("20").Uint64Var(&params.TopN)
	return params
}

func queryNovelty(ctx context.Context, params *queryNoveltyParams) (err error) {
	from, to, err := params.parseFromTo()
	if err != nil {
		return err
	}

	samples := novelty.NewSamples(0, 0.05)

	pos := from

	for {
		pos = pos.Add(params.StepSize)
		from := pos.Add(-params.StepSize)

		if pos.After(to) {
			break
		}

		level.Info(logger).Log("msg", "query profile", "url", params.URL, "from", from, "to", to)

		req := &querierv1.SelectMergeProfileRequest{
			ProfileTypeID: params.ProfileType,
			Start:         from.UnixMilli(),
			End:           pos.UnixMilli(),
			LabelSelector: params.Query,
		}

		qc := params.phlareClient.queryClient()
		resp, err := qc.SelectMergeProfile(ctx, connect.NewRequest(req))
		if err != nil {
			return errors.Wrap(err, "failed to query")
		}

		// sort the samples by the first type
		profileTypeIdx := 0
		sort.Slice(resp.Msg.Sample, func(i, j int) bool {
			return resp.Msg.Sample[i].Value[profileTypeIdx] > resp.Msg.Sample[j].Value[profileTypeIdx]
		})

		// append the first N samples to the noveltySamples
		end := len(resp.Msg.Sample)
		if end > int(params.TopN) {
			end = int(params.TopN)
		}
		stacks := make([]string, 0, end)
		values := make([]int64, 0, end)
		for _, sample := range resp.Msg.Sample[:end] {
			stackParts := make([]string, len(sample.LocationId))
			for idx := range sample.LocationId {
				loc := resp.Msg.Location[sample.LocationId[idx]-1]
				if len(loc.Line) == 0 {
					panic("no line")
				}
				functionID := loc.Line[0].FunctionId
				function := resp.Msg.Function[functionID-1]
				stackParts[idx] = resp.Msg.StringTable[function.Name]
			}
			stacks = append(stacks, strings.Join(stackParts, "|"))
			values = append(values, sample.Value[profileTypeIdx])
		}

		for idx, name := range stacks {
			fmt.Printf("%d: %s\n", idx, name)
			fmt.Printf("%d: %s\n", idx, values[idx])
		}

		noveltyScore := samples.Add(stacks, values)

		log.Printf("novelty score: %f", noveltyScore)
	}

	return nil
}
