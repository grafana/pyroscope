package symdb_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/grafana/pyroscope/v2/pkg/model"
	schemav1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/v2/pkg/pprof"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

// BenchmarkFunctionProjectionSizes includes sample collection, symbol resolution,
// aggregation, response construction and protobuf JSON encoding. Input subsets
// share one in-memory symbol partition; storage I/O, planning and network are
// excluded. Node counts are measured from the complete tree, not max_nodes.
// The fixture's obfuscated names affect JSON escaping and response sizes.
// Run with go test ./pkg/phlaredb/symdb -run '^$'
// -bench BenchmarkFunctionProjectionSizes -benchmem -benchtime=500ms -count=3.
// Compare median ns/op, B/op and json-B/result for each input size.
func BenchmarkFunctionProjectionSizes(b *testing.B) {
	profile, err := pprof.OpenFile("testdata/big-profile.pb.gz")
	require.NoError(b, err)
	profile.Normalize()
	db := symdb.NewSymDB(symdb.DefaultConfig().WithDirectory(b.TempDir()))
	all := db.WriteProfileSymbols(0, profile.Profile)[0].Samples
	order := rand.New(rand.NewPCG(42, 17)).Perm(len(all.Values))
	for _, count := range []int{100, 1000, 10000, len(all.Values)} {
		if count > len(all.Values) {
			continue
		}
		samples := schemav1.NewSamples(count)
		for i, index := range order[:count] {
			samples.StacktraceIDs[i], samples.Values[i] = all.StacktraceIDs[index], all.Values[index]
		}
		full := symdb.NewResolver(context.Background(), db, symdb.WithResolverMaxNodes(0))
		full.AddSamples(0, samples)
		tree, err := full.Tree()
		full.Release()
		require.NoError(b, err)
		fg := model.NewFlameGraph(tree, 0)
		nodes := -1 // Exclude the synthetic profile root.
		for _, level := range fg.Levels {
			nodes += len(level.Values) / 4
		}
		cases := []projectionBenchCase{
			{
				name:   "flamegraph_full",
				format: querierv1.ProfileFormat_PROFILE_FORMAT_FLAMEGRAPH,
			},
			{
				name:   "tree_full",
				format: querierv1.ProfileFormat_PROFILE_FORMAT_TREE,
			},
			{
				name:   "functions_all",
				format: querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS,
			},
			{
				name:   "functions_top100",
				format: querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS,
				limit:  100,
			},
		}
		b.Run(fmt.Sprintf("nodes_%d", nodes), func(b *testing.B) {
			for _, tc := range cases {
				b.Run(tc.name, func(b *testing.B) {
					run := func() (*querierv1.SelectMergeStacktracesResponse, []byte, error) {
						r := symdb.NewResolver(context.Background(), db, symdb.WithResolverMaxNodes(0))
						defer r.Release()
						r.AddSamples(0, samples)
						resp, err := tc.response(r)
						if err != nil {
							return nil, nil, err
						}
						data, err := protojson.Marshal(resp)
						return resp, data, err
					}
					// Warm caches/pools and collect result metrics outside the timed loop.
					resp, data, err := run()
					require.NoError(b, err)
					if tc.format == querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS {
						require.Equal(b, int64(samples.Sum()), resp.Functions.Total)
					}
					if tc.format == querierv1.ProfileFormat_PROFILE_FORMAT_FLAMEGRAPH {
						require.Equal(b, int64(samples.Sum()), resp.Flamegraph.Total)
					}
					responseBytes := len(data)
					outputNodes := projectionResponseNodes(resp)
					b.ReportAllocs()
					for b.Loop() {
						_, _, err := run()
						if err != nil {
							b.Fatal(err)
						}
					}
					b.ReportMetric(float64(nodes), "input-nodes")
					b.ReportMetric(float64(count), "input-stacks")
					b.ReportMetric(float64(responseBytes), "json-B/result")
					if outputNodes >= 0 {
						b.ReportMetric(float64(outputNodes), "output-nodes")
					}
				})
			}
		})
	}
}

type projectionBenchCase struct {
	name   string
	format querierv1.ProfileFormat
	limit  int
}

func (c projectionBenchCase) response(r *symdb.Resolver) (*querierv1.SelectMergeStacktracesResponse, error) {
	resp := new(querierv1.SelectMergeStacktracesResponse)
	switch c.format {
	case querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS:
		functions, err := r.Functions()
		if err != nil {
			return nil, err
		}
		resp.Functions = functions
		if c.limit > 0 && len(resp.Functions.Functions) > c.limit {
			resp.Functions.Functions = resp.Functions.Functions[:c.limit]
		}
	default:
		tree, err := r.Tree()
		if err != nil {
			return nil, err
		}
		encoded := tree.Bytes(0, nil)
		if c.format == querierv1.ProfileFormat_PROFILE_FORMAT_TREE {
			resp.Tree = encoded
		} else {
			// The real frontend decodes the backend tree before producing a flame graph.
			tree, err = model.UnmarshalTree[model.FunctionName, model.FunctionNameI](encoded)
			if err != nil {
				return nil, err
			}
			resp.Flamegraph = model.NewFlameGraph(tree, 0)
		}
	}
	return resp, nil
}

func projectionResponseNodes(resp *querierv1.SelectMergeStacktracesResponse) int {
	if resp.Functions != nil {
		return len(resp.Functions.Functions)
	}
	if resp.Flamegraph != nil {
		n := 0
		for _, level := range resp.Flamegraph.Levels {
			n += len(level.Values) / 4
		}
		return n
	}
	return -1
}
