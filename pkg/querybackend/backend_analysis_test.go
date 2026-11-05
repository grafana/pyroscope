package querybackend

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func TestQueryBackend_AnalyzeSeriesOnlyAtRoot(t *testing.T) {
	// The spike is at the start of the second block: detection needs the
	// baseline from the first block and confirmation from the second.
	blocks := []*metastorev1.BlockMeta{{Id: "0"}, {Id: "1"}, {Id: "2"}}
	read := func(blocks ...*metastorev1.BlockMeta) *queryv1.QueryNode {
		return &queryv1.QueryNode{Type: queryv1.QueryNode_READ, Blocks: blocks}
	}
	merge := func(children ...*queryv1.QueryNode) *queryv1.QueryNode {
		return &queryv1.QueryNode{Type: queryv1.QueryNode_MERGE, Children: children}
	}
	plans := map[string]*queryv1.QueryNode{
		"read root":     read(blocks...),
		"merge root":    merge(read(blocks[0]), read(blocks[1]), read(blocks[2])),
		"nested merges": merge(merge(read(blocks[0]), read(blocks[1])), read(blocks[2])),
	}
	for name, root := range plans {
		t.Run(name, func(t *testing.T) {
			query := &queryv1.TimeSeriesAnalysisQuery{
				Step: 1, Limit: 1,
				Config: &queryv1.TimeSeriesAnalysisConfig{
					BaselineWindow: 3, ConfirmationWindow: 3, MinimumSustainedPoints: 2,
				},
			}
			reader := &testQueryHandler{invoke: func(_ context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
				agg := newAggregator(req)
				for _, block := range req.QueryPlan.Root.Blocks {
					var blockIndex int
					if _, err := fmt.Sscanf(block.Id, "%d", &blockIndex); err != nil {
						return nil, err
					}
					var series []*typesv1.Series
					for _, name := range []string{"weak", "strong"} {
						s := &typesv1.Series{Labels: []*typesv1.LabelPair{{Name: "service_name", Value: name}}}
						for i := blockIndex * 7; i < (blockIndex+1)*7; i++ {
							value := 100.0
							if i == 7 {
								value = 200
								if name == "strong" {
									value = 300
								}
							}
							s.Points = append(s.Points, &typesv1.Point{Timestamp: int64(i+1) * 1000, Value: value})
						}
						series = append(series, s)
					}
					if err := agg.aggregateReport(&queryv1.Report{
						ReportType:         queryv1.ReportType_REPORT_TIME_SERIES_ANALYSIS,
						TimeSeriesAnalysis: &queryv1.TimeSeriesAnalysisReport{Query: query.CloneVT(), TimeSeries: series},
					}); err != nil {
						return nil, err
					}
				}
				resp := agg.response()
				assert.Empty(t, resp.Reports[0].TimeSeriesAnalysis.Events)
				assert.Len(t, resp.Reports[0].TimeSeriesAnalysis.TimeSeries, 2)
				return resp, nil
			}}
			backend, err := New(Config{}, log.NewNopLogger(), nil, nil, reader)
			require.NoError(t, err)
			backend.backendClient = &testQueryHandler{invoke: func(ctx context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
				assert.False(t, req.GetOptions().GetFinalize())
				// Other options must survive propagation to child invocations.
				assert.True(t, req.GetOptions().GetCollectDiagnostics())
				resp, err := backend.Invoke(ctx, req)
				if err == nil {
					assert.Empty(t, resp.Reports[0].TimeSeriesAnalysis.Events)
					assert.Len(t, resp.Reports[0].TimeSeriesAnalysis.TimeSeries, 2)
				}
				return resp, err
			}}
			resp, err := backend.Invoke(context.Background(), &queryv1.InvokeRequest{
				StartTime: 0, EndTime: 21000,
				QueryPlan: &queryv1.QueryPlan{Root: root},
				Options:   &queryv1.InvokeOptions{Finalize: true, CollectDiagnostics: true},
			})
			require.NoError(t, err)
			require.Len(t, resp.Reports, 1)
			result := resp.Reports[0].TimeSeriesAnalysis
			require.Empty(t, result.TimeSeries)
			require.Len(t, result.Events, 1)
			event := result.Events[0]
			assert.Equal(t, "strong", event.Labels[0].Value)
			assert.Equal(t, int64(8000), event.TimestampStart)
			assert.Equal(t, 100.0, event.Baseline)
			assert.Equal(t, 300.0, event.PeakValue)
			assert.Equal(t, querierv1.TimeSeriesAnomalyType_TIME_SERIES_ANOMALY_TYPE_SPIKE, event.Type)
		})
	}
}
