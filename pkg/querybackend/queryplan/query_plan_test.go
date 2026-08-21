package queryplan

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

var update = flag.Bool("update", false, "rewrite golden files in testdata/ from the current plan output")

// Test_Build verifies the shape of query plans produced by Build and
// BuildBalanced against golden files in testdata/, running both builders over
// the same input table so the two algorithms are easy to compare. Each
// subtest's golden file is named after the subtest: Build's golden file is
// named after the case (e.g. Test_Build/single_block reads
// testdata/single_block.txt), and BuildBalanced's is named after the case
// with a "balanced_" prefix (e.g. Test_Build/balanced_single_block reads
// testdata/balanced_single_block.txt).
//
// To regenerate all golden files:
//
//	go test ./pkg/querybackend/queryplan/ -update
//
// To regenerate a specific golden file:
//
//	go test ./pkg/querybackend/queryplan/ -run 'Test_Build/<name>$' -update
func Test_Build(t *testing.T) {
	tests := []struct {
		name      string
		blocks    int
		maxReads  int
		maxMerges int
	}{
		{name: "empty", blocks: 0, maxReads: 2, maxMerges: 3},
		{name: "invalid_max_reads", blocks: 10, maxReads: 0, maxMerges: 3},
		{name: "invalid_max_merges", blocks: 10, maxReads: 2, maxMerges: 1},
		{name: "single_block", blocks: 1, maxReads: 2, maxMerges: 3},
		{name: "exact_one_leaf", blocks: 2, maxReads: 2, maxMerges: 3},
		{name: "two_leaves", blocks: 3, maxReads: 2, maxMerges: 3},
		{name: "full_depth_2", blocks: 6, maxReads: 2, maxMerges: 3},
		{name: "just_over_depth_2", blocks: 7, maxReads: 2, maxMerges: 3},
		{name: "twenty_five_blocks", blocks: 25, maxReads: 2, maxMerges: 3},
		{name: "full_merge_vs_single_leaf_merge", blocks: 33, maxReads: 4, maxMerges: 8},
		{name: "forced_equal_split", blocks: 16, maxReads: 1, maxMerges: 8},
		{name: "three_way_split", blocks: 20, maxReads: 1, maxMerges: 8},
	}

	builders := []struct {
		name   string
		build  func(blocks []*metastorev1.BlockMeta, maxReads, maxMerges int) *queryv1.QueryPlan
		prefix string
	}{
		{name: "Build", build: Build, prefix: "build_"},
		{name: "BuildBalanced", build: BuildBalanced, prefix: "balanced_"},
	}

	for _, tt := range tests {
		for _, b := range builders {
			t.Run(b.prefix+tt.name, func(t *testing.T) {
				blocks := makeBlocks(tt.blocks)
				p := b.build(blocks, tt.maxReads, tt.maxMerges)

				var buf bytes.Buffer
				writePlan(t, &buf, "", p.Root)

				// Ensure that the plan has not been modified during traversal.
				assert.Equal(t, b.build(blocks, tt.maxReads, tt.maxMerges), p)

				if *update {
					require.NoError(t, os.WriteFile(goldenFile(t), buf.Bytes(), 0o644))
					return
				}

				expected, err := os.ReadFile(goldenFile(t))
				require.NoError(t, err)
				assert.Equal(t, string(expected), buf.String())
			})
		}
	}
}

// makeBlocks creates n BlockMeta with sequential string IDs starting at "1".
func makeBlocks(n int) []*metastorev1.BlockMeta {
	blocks := make([]*metastorev1.BlockMeta, n)
	for i := range n {
		blocks[i] = &metastorev1.BlockMeta{Id: strconv.Itoa(i + 1)}
	}
	return blocks
}

// goldenFile returns the testdata path for the current (sub)test. The file
// name is the last segment of t.Name(). For `Test_Build/single_block` it
// returns `testdata/single_block.txt`.
func goldenFile(t *testing.T) string {
	t.Helper()
	parts := strings.Split(t.Name(), "/")
	return filepath.Join("testdata", parts[len(parts)-1]+".txt")
}

// Test_balanceGroupItems verifies that items are spread evenly across groups,
// with any remainder distributed one-per-group starting from group 0.
func Test_balanceGroupItems(t *testing.T) {
	tests := []struct {
		name      string
		numItems  int
		numGroups int
		want      []int // want[i] is the expected size of group i
	}{
		{name: "zero_items", numItems: 0, numGroups: 3, want: []int{0, 0, 0}},
		{name: "single_group", numItems: 5, numGroups: 1, want: []int{5}},
		{name: "even_split", numItems: 6, numGroups: 3, want: []int{2, 2, 2}},
		{name: "remainder_spread", numItems: 5, numGroups: 3, want: []int{2, 2, 1}},
		{name: "remainder_almost_full", numItems: 8, numGroups: 3, want: []int{3, 3, 2}},
		{name: "one_item_per_group", numItems: 3, numGroups: 3, want: []int{1, 1, 1}},
		{name: "more_groups_than_items", numItems: 2, numGroups: 5, want: []int{1, 1, 0, 0, 0}},
		{name: "no_items", numItems: 0, numGroups: 1, want: []int{0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Len(t, tt.want, tt.numGroups, "test case is malformed: want must have numGroups elements")

			got := make([]int, tt.numGroups)
			sum := 0
			for idx := range tt.numGroups {
				got[idx] = balanceGroupItems(tt.numItems, tt.numGroups, idx)
				sum += got[idx]
			}

			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.numItems, sum, "group sizes must sum back to numItems")
		})
	}
}

// writePlan writes an indented textual representation of the plan rooted at
// n to w. A nil root produces no output. The test fails on malformed nodes.
func writePlan(t *testing.T, w io.Writer, pad string, n *queryv1.QueryNode) {
	t.Helper()
	if n == nil {
		return
	}
	fmt.Fprintf(w, pad+"%s {children: %d, blocks: %d}\n",
		n.Type, len(n.Children), len(n.Blocks))
	switch n.Type {
	case queryv1.QueryNode_MERGE:
		for _, child := range n.Children {
			writePlan(t, w, pad+"\t", child)
		}
	case queryv1.QueryNode_READ:
		for _, md := range n.Blocks {
			fmt.Fprintf(w, pad+"\t"+"id:\"%s\"\n", md.Id)
		}
	default:
		t.Fatalf("unknown node type: %v", n.Type)
	}
}
