package queryplan

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

func testingPlan(w io.Writer, pad string, n *queryv1.QueryNode, debug bool) error {
	if debug {
		_, _ = fmt.Fprintf(w, pad+"%s {children: %d, blocks: %d}\n",
			n.Type, len(n.Children), len(n.Blocks))
	} else {
		_, _ = fmt.Fprintf(w, pad+"%s (%d)\n", n.Type, len(n.Children))
	}

	switch n.Type {
	case queryv1.QueryNode_MERGE:
		for _, child := range n.Children {
			err := testingPlan(w, pad+"\t", child, debug)
			if err != nil {
				return err
			}
		}

	case queryv1.QueryNode_READ:
		for _, md := range n.Blocks {
			_, _ = fmt.Fprintf(w, pad+"\t"+"id:\"%s\"\n", md.Id)
		}

	default:
		return fmt.Errorf("unknown type: %T", n.Type)
	}
	return nil
}

func Test_Plan(t *testing.T) {
	blocks := []*metastorev1.BlockMeta{
		{Id: "1"}, {Id: "2"},
		{Id: "3"}, {Id: "4"},
		{Id: "5"}, {Id: "6"},
		{Id: "7"}, {Id: "8"},
		{Id: "9"}, {Id: "10"},
		{Id: "11"}, {Id: "12"},
		{Id: "13"}, {Id: "14"},
		{Id: "15"}, {Id: "16"},
		{Id: "17"}, {Id: "18"},
		{Id: "19"}, {Id: "20"},
		{Id: "21"}, {Id: "22"},
		{Id: "23"}, {Id: "24"},
		{Id: "25"},
	}

	p := Build(blocks, 2, 3)
	var buf bytes.Buffer
	err := testingPlan(&buf, "", p.Root, true)
	require.NoError(t, err)

	// Ensure that the plan has not been modified
	// during traversal performed by printPlan.
	assert.Equal(t, Build(blocks, 2, 3), p)

	expected, err := os.ReadFile("testdata/plan.txt")
	require.NoError(t, err)
	assert.Equal(t, string(expected), buf.String())
}
