package main

import (
	"context"
	"sort"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/dustin/go-humanize"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/pkg/pprof/novelty"
)

type queryNoveltyParams struct {
	*queryParams
	TopN           uint64        // TOPk stacktraces
	StepSize       time.Duration // Step size for the novelty query
	ProfileType    string
	Dimension      string
	MergeThreshold float64
}

func addQueryNoveltyParams(queryCmd commander) *queryNoveltyParams {
	params := new(queryNoveltyParams)
	params.queryParams = addQueryParams(queryCmd)
	queryCmd.Flag("profile-type", "Profile type to query.").Default("process_cpu:cpu:nanoseconds:cpu:nanoseconds").StringVar(&params.ProfileType)
	queryCmd.Flag("step-size", "Show the top N stacktraces").Default("15s").DurationVar(&params.StepSize)
	queryCmd.Flag("top-n", "Show the top N").Default("20").Uint64Var(&params.TopN)
	queryCmd.Flag("dimension", "Aggregate stacktrace-self or function-self").Default("function-self").StringVar(&params.Dimension)
	queryCmd.Flag("merge-threshold", "Threshold when to consider profiles simliar enough").Default("0.10").Float64Var(&params.MergeThreshold)
	return params
}

func queryNovelty(ctx context.Context, params *queryNoveltyParams) (err error) {
	from, to, err := params.parseFromTo()
	if err != nil {
		return err
	}

	samples := novelty.NewSamples(0, params.MergeThreshold)

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
		var stacks []string
		var values []int64
		profileTypeIdx := 0

		if params.Dimension == "stacktrace-self" {
			// sort the samples by the first type
			sort.Slice(resp.Msg.Sample, func(i, j int) bool {
				return resp.Msg.Sample[i].Value[profileTypeIdx] > resp.Msg.Sample[j].Value[profileTypeIdx]
			})
			// append the first N samples to the noveltySamples
			end := len(resp.Msg.Sample)
			if end > int(params.TopN) {
				end = int(params.TopN)
			}
			stacks = make([]string, 0, end)
			values = make([]int64, 0, end)
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
		} else if params.Dimension == "function-self" {
			values = make([]int64, len(resp.Msg.Function))

			// add the leaf function to the values
			for _, sample := range resp.Msg.Sample {
				if len(sample.LocationId) == 0 {
					continue
				}

				leafLoc := resp.Msg.Location[sample.LocationId[0]-1]
				functionID := leafLoc.Line[0].FunctionId
				values[functionID-1] += sample.Value[profileTypeIdx]
			}

			// sort the functions and values by the biggest
			sort.Slice(resp.Msg.Function, func(i, j int) bool {
				return values[i] > values[j]
			})
			sort.Slice(values, func(i, j int) bool {
				return values[i] > values[j]
			})

			// append the first N samples to the noveltySamples
			end := len(resp.Msg.Function)
			if end > int(params.TopN) {
				end = int(params.TopN)
			}

			// resovle to func names
			stacks = make([]string, end)
			for i := range stacks {
				stacks[i] = resp.Msg.StringTable[resp.Msg.Function[i].Name]
			}
			values = values[:end]
		} else {
			return errors.New("invalid dimension: " + params.Dimension)
		}

		for i, stack := range stacks {
			level.Debug(logger).Log("stack", stack, "value", humanize.FormatInteger("", int(values[i])))
		}

		noveltyScore := samples.Add(stacks, values)

		level.Info(logger).Log("novelty score", noveltyScore)
	}

	return nil
}
