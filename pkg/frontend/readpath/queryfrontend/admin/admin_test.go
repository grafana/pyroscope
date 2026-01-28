package admin

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

func TestParseQueryType(t *testing.T) {
	a := &Admin{}

	tests := []struct {
		input    string
		expected queryv1.QueryType
	}{
		{"QUERY_PPROF", queryv1.QueryType_QUERY_PPROF},
		{"QUERY_TREE", queryv1.QueryType_QUERY_TREE},
		{"QUERY_TIME_SERIES", queryv1.QueryType_QUERY_TIME_SERIES},
		{"QUERY_HEATMAP", queryv1.QueryType_QUERY_HEATMAP},
		{"QUERY_LABEL_NAMES", queryv1.QueryType_QUERY_LABEL_NAMES},
		{"QUERY_LABEL_VALUES", queryv1.QueryType_QUERY_LABEL_VALUES},
		{"QUERY_SERIES_LABELS", queryv1.QueryType_QUERY_SERIES_LABELS},
		{"", queryv1.QueryType_QUERY_PPROF},           // default
		{"INVALID", queryv1.QueryType_QUERY_PPROF},   // default
		{"query_pprof", queryv1.QueryType_QUERY_PPROF}, // case sensitive, defaults
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := a.parseQueryType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseIntOrDefault(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		defaultVal int64
		expected   int64
	}{
		{"valid positive", "42", 0, 42},
		{"valid negative", "-10", 0, -10},
		{"valid zero", "0", 99, 0},
		{"empty string", "", 100, 100},
		{"invalid string", "abc", 50, 50},
		{"float string", "3.14", 0, 0}, // ParseInt fails on floats
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseIntOrDefault(tt.input, tt.defaultVal)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseFloatOrDefault(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		defaultVal float64
		expected   float64
	}{
		{"valid integer", "42", 0, 42.0},
		{"valid float", "3.14", 0, 3.14},
		{"valid negative", "-2.5", 0, -2.5},
		{"empty string", "", 1.5, 1.5},
		{"invalid string", "abc", 2.5, 2.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseFloatOrDefault(tt.input, tt.defaultVal)
			assert.InDelta(t, tt.expected, result, 0.001)
		})
	}
}

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"simple", "a,b,c", []string{"a", "b", "c"}},
		{"with spaces", "a, b , c", []string{"a", "b", "c"}},
		{"single item", "foo", []string{"foo"}},
		{"empty string", "", []string{}},
		{"empty items", "a,,b", []string{"a", "b"}},
		{"only spaces", " , , ", []string{}},
		{"trailing comma", "a,b,", []string{"a", "b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitAndTrim(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertQueryPlanToTree(t *testing.T) {
	t.Run("nil plan", func(t *testing.T) {
		result := convertQueryPlanToTree(nil)
		assert.Nil(t, result)
	})

	t.Run("nil root", func(t *testing.T) {
		plan := &queryv1.QueryPlan{Root: nil}
		result := convertQueryPlanToTree(plan)
		assert.Nil(t, result)
	})

	t.Run("simple READ node", func(t *testing.T) {
		plan := &queryv1.QueryPlan{
			Root: &queryv1.QueryNode{
				Type: queryv1.QueryNode_READ,
				Blocks: []*metastorev1.BlockMeta{
					{Id: "block-1", Shard: 1, Size: 1024, CompactionLevel: 0},
					{Id: "block-2", Shard: 1, Size: 2048, CompactionLevel: 1},
				},
			},
		}

		result := convertQueryPlanToTree(plan)
		require.NotNil(t, result)
		assert.Equal(t, "READ", result.Type)
		assert.Equal(t, 2, result.BlockCount)
		assert.Equal(t, 2, result.TotalBlocks)
		assert.Len(t, result.Blocks, 2)
		assert.Equal(t, "block-1", result.Blocks[0].ID)
	})

	t.Run("MERGE with children", func(t *testing.T) {
		plan := &queryv1.QueryPlan{
			Root: &queryv1.QueryNode{
				Type: queryv1.QueryNode_MERGE,
				Children: []*queryv1.QueryNode{
					{
						Type: queryv1.QueryNode_READ,
						Blocks: []*metastorev1.BlockMeta{
							{Id: "block-1", Shard: 1, Size: 1024},
						},
					},
					{
						Type: queryv1.QueryNode_READ,
						Blocks: []*metastorev1.BlockMeta{
							{Id: "block-2", Shard: 2, Size: 2048},
							{Id: "block-3", Shard: 2, Size: 3072},
						},
					},
				},
			},
		}

		result := convertQueryPlanToTree(plan)
		require.NotNil(t, result)
		assert.Equal(t, "MERGE", result.Type)
		assert.Equal(t, 3, result.TotalBlocks)
		assert.Len(t, result.Children, 2)
		assert.Equal(t, "READ", result.Children[0].Type)
		assert.Equal(t, 1, result.Children[0].TotalBlocks)
		assert.Equal(t, "READ", result.Children[1].Type)
		assert.Equal(t, 2, result.Children[1].TotalBlocks)
	})

	t.Run("READ node limits displayed blocks to 5", func(t *testing.T) {
		blocks := make([]*metastorev1.BlockMeta, 10)
		for i := 0; i < 10; i++ {
			blocks[i] = &metastorev1.BlockMeta{Id: "block", Shard: uint32(i), Size: 1024}
		}

		plan := &queryv1.QueryPlan{
			Root: &queryv1.QueryNode{
				Type:   queryv1.QueryNode_READ,
				Blocks: blocks,
			},
		}

		result := convertQueryPlanToTree(plan)
		require.NotNil(t, result)
		assert.Equal(t, 10, result.BlockCount)
		assert.Len(t, result.Blocks, 5) // limited to 5 for display
	})
}

func TestConvertExecutionNodeToTree(t *testing.T) {
	t.Run("nil node", func(t *testing.T) {
		result := convertExecutionNodeToTree(nil)
		assert.Nil(t, result)
	})

	t.Run("simple READ node", func(t *testing.T) {
		baseTime := int64(1000000000) // 1 second in nanoseconds
		node := &queryv1.ExecutionNode{
			Type:        queryv1.QueryNode_READ,
			Executor:    "host-1",
			StartTimeNs: baseTime,
			EndTimeNs:   baseTime + 500000000, // 500ms later
			Stats: &queryv1.ExecutionStats{
				BlocksRead:        5,
				DatasetsProcessed: 10,
			},
		}

		result := convertExecutionNodeToTree(node)
		require.NotNil(t, result)
		assert.Equal(t, "READ", result.Type)
		assert.Equal(t, "host-1", result.Executor)
		assert.Equal(t, 500*time.Millisecond, result.Duration)
		assert.Equal(t, "500ms", result.DurationStr)
		assert.Equal(t, time.Duration(0), result.RelativeStart)
		require.NotNil(t, result.Stats)
		assert.Equal(t, int64(5), result.Stats.BlocksRead)
		assert.Equal(t, int64(10), result.Stats.DatasetsProcessed)
	})

	t.Run("MERGE with children", func(t *testing.T) {
		baseTime := int64(1000000000)
		node := &queryv1.ExecutionNode{
			Type:        queryv1.QueryNode_MERGE,
			Executor:    "host-1",
			StartTimeNs: baseTime,
			EndTimeNs:   baseTime + 1000000000, // 1s later
			Children: []*queryv1.ExecutionNode{
				{
					Type:        queryv1.QueryNode_READ,
					Executor:    "host-2",
					StartTimeNs: baseTime + 100000000,  // started 100ms after parent
					EndTimeNs:   baseTime + 600000000,  // 500ms duration
				},
				{
					Type:        queryv1.QueryNode_READ,
					Executor:    "host-3",
					StartTimeNs: baseTime + 200000000,  // started 200ms after parent
					EndTimeNs:   baseTime + 800000000,  // 600ms duration
				},
			},
		}

		result := convertExecutionNodeToTree(node)
		require.NotNil(t, result)
		assert.Equal(t, "MERGE", result.Type)
		assert.Len(t, result.Children, 2)

		// Children should have relative start times based on the earliest time in the tree
		assert.Equal(t, 100*time.Millisecond, result.Children[0].RelativeStart)
		assert.Equal(t, 200*time.Millisecond, result.Children[1].RelativeStart)
	})

	t.Run("node with error", func(t *testing.T) {
		node := &queryv1.ExecutionNode{
			Type:        queryv1.QueryNode_READ,
			Executor:    "host-1",
			StartTimeNs: 0,
			EndTimeNs:   100000000,
			Error:       "something went wrong",
		}

		result := convertExecutionNodeToTree(node)
		require.NotNil(t, result)
		assert.Equal(t, "something went wrong", result.Error)
	})

	t.Run("node with block executions", func(t *testing.T) {
		baseTime := int64(1000000000)
		node := &queryv1.ExecutionNode{
			Type:        queryv1.QueryNode_READ,
			Executor:    "host-1",
			StartTimeNs: baseTime,
			EndTimeNs:   baseTime + 500000000,
			Stats: &queryv1.ExecutionStats{
				BlocksRead:        2,
				DatasetsProcessed: 4,
				BlockExecutions: []*queryv1.BlockExecution{
					{
						BlockId:           "block-1",
						StartTimeNs:       baseTime + 10000000,
						EndTimeNs:         baseTime + 200000000,
						DatasetsProcessed: 2,
						Size:              1024,
						Shard:             1,
						CompactionLevel:   0,
					},
					{
						BlockId:           "block-2",
						StartTimeNs:       baseTime + 210000000,
						EndTimeNs:         baseTime + 450000000,
						DatasetsProcessed: 2,
						Size:              2048,
						Shard:             1,
						CompactionLevel:   1,
					},
				},
			},
		}

		result := convertExecutionNodeToTree(node)
		require.NotNil(t, result)
		require.NotNil(t, result.Stats)
		require.Len(t, result.Stats.BlockExecutions, 2)

		block1 := result.Stats.BlockExecutions[0]
		assert.Equal(t, "block-1", block1.BlockID)
		assert.Equal(t, 190*time.Millisecond, block1.Duration)
		assert.Equal(t, int64(2), block1.DatasetsProcessed)
		assert.Equal(t, "1.0 kB", block1.Size)
		assert.Equal(t, uint32(1), block1.Shard)
		assert.Equal(t, uint32(0), block1.CompactionLevel)
	})
}

func TestBuildMetadataStats(t *testing.T) {
	startTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	endTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	t.Run("empty blocks", func(t *testing.T) {
		result := buildMetadataStats(nil, startTime, endTime)
		assert.Contains(t, result, "Blocks found: 0")
		assert.Contains(t, result, "Time range:")
	})

	t.Run("with blocks", func(t *testing.T) {
		blocks := []*metastorev1.BlockMeta{
			{
				Id:              "block-1",
				Shard:          1,
				Size:           1024,
				CompactionLevel: 0,
				Datasets: []*metastorev1.Dataset{
					{Size: 512},
					{Size: 512},
				},
			},
			{
				Id:              "block-2",
				Shard:          2,
				Size:           2048,
				CompactionLevel: 1,
				Datasets: []*metastorev1.Dataset{
					{Size: 2048},
				},
			},
		}

		result := buildMetadataStats(blocks, startTime, endTime)
		assert.Contains(t, result, "Blocks found: 2")
		assert.Contains(t, result, "Block Statistics:")
		assert.Contains(t, result, "Total size:")
		assert.Contains(t, result, "Dataset Statistics:")
		assert.Contains(t, result, "Total datasets: 3")
	})
}

func TestFindEarliestStartTime(t *testing.T) {
	t.Run("nil node", func(t *testing.T) {
		result := findEarliestStartTime(nil)
		assert.Equal(t, int64(0), result)
	})

	t.Run("single node", func(t *testing.T) {
		node := &queryv1.ExecutionNode{
			StartTimeNs: 1000,
		}
		result := findEarliestStartTime(node)
		assert.Equal(t, int64(1000), result)
	})

	t.Run("node with earlier block execution", func(t *testing.T) {
		node := &queryv1.ExecutionNode{
			StartTimeNs: 1000,
			Stats: &queryv1.ExecutionStats{
				BlockExecutions: []*queryv1.BlockExecution{
					{StartTimeNs: 500},
					{StartTimeNs: 800},
				},
			},
		}
		result := findEarliestStartTime(node)
		assert.Equal(t, int64(500), result)
	})

	t.Run("node with earlier child", func(t *testing.T) {
		node := &queryv1.ExecutionNode{
			StartTimeNs: 1000,
			Children: []*queryv1.ExecutionNode{
				{StartTimeNs: 900},
				{StartTimeNs: 1100},
			},
		}
		result := findEarliestStartTime(node)
		assert.Equal(t, int64(900), result)
	})
}

func TestExtractBlocksFromPlan(t *testing.T) {
	t.Run("nil plan", func(t *testing.T) {
		result := extractBlocksFromPlan(nil)
		assert.Nil(t, result)
	})

	t.Run("nil root", func(t *testing.T) {
		plan := &queryv1.QueryPlan{}
		result := extractBlocksFromPlan(plan)
		assert.Nil(t, result)
	})

	t.Run("single READ node", func(t *testing.T) {
		plan := &queryv1.QueryPlan{
			Root: &queryv1.QueryNode{
				Type: queryv1.QueryNode_READ,
				Blocks: []*metastorev1.BlockMeta{
					{Id: "block-1"},
					{Id: "block-2"},
				},
			},
		}
		result := extractBlocksFromPlan(plan)
		assert.Len(t, result, 2)
	})

	t.Run("nested MERGE nodes", func(t *testing.T) {
		plan := &queryv1.QueryPlan{
			Root: &queryv1.QueryNode{
				Type: queryv1.QueryNode_MERGE,
				Children: []*queryv1.QueryNode{
					{
						Type:   queryv1.QueryNode_READ,
						Blocks: []*metastorev1.BlockMeta{{Id: "block-1"}},
					},
					{
						Type: queryv1.QueryNode_MERGE,
						Children: []*queryv1.QueryNode{
							{
								Type:   queryv1.QueryNode_READ,
								Blocks: []*metastorev1.BlockMeta{{Id: "block-2"}, {Id: "block-3"}},
							},
						},
					},
				},
			},
		}
		result := extractBlocksFromPlan(plan)
		assert.Len(t, result, 3)
	})
}
