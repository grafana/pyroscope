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

// Test_Build verifies the shape of query plans produced by Build against
// golden files in testdata/. Each subtest's golden file is named after the
// subtest. E.g. Test_Build/single_block reads testdata/single_block.txt.
//
// To regenerate all golden files:
//
//	go test ./pkg/querybackend/queryplan/ -update
//
// To regenerate a specific golden file:
//
//	go test ./pkg/querybackend/queryplan/ -run Test_Build/<name> -update
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := makeBlocks(tt.blocks)
			p := Build(blocks, tt.maxReads, tt.maxMerges)

			var buf bytes.Buffer
			writePlan(t, &buf, "", p.Root)

			// Ensure that the plan has not been modified during traversal.
			assert.Equal(t, Build(blocks, tt.maxReads, tt.maxMerges), p)

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

// makeBlocks creates n BlockMeta with sequential string IDs starting at "1".
func makeBlocks(n int) []*metastorev1.BlockMeta {
	blocks := make([]*metastorev1.BlockMeta, n)
	for i := 0; i < n; i++ {
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
