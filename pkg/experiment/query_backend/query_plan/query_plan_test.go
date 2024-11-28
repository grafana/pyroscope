package query_plan

import (
	"bytes"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

func Test_Plan(t *testing.T) {
	block := func(i int) *metastorev1.BlockMeta {
		return &metastorev1.BlockMeta{Id: 1, StringTable: []string{"", strconv.Itoa(i)}}
	}
	blocks := []*metastorev1.BlockMeta{
		block(1), block(2),
		block(3), block(4),
		block(5), block(6),
		block(7), block(8),
		block(9), block(10),
		block(11), block(12),
		block(13), block(14),
		block(15), block(16),
		block(17), block(18),
		block(19), block(20),
		block(21), block(22),
		block(23), block(24),
		block(25),
	}

	p := Build(blocks, 2, 3)
	var buf bytes.Buffer
	printPlan(&buf, "", p.Root, true)
	// Ensure that the plan has not been modified
	// during traversal performed by printPlan.
	assert.Equal(t, Build(blocks, 2, 3), p)

	expected, err := os.ReadFile("testdata/plan.txt")
	require.NoError(t, err)
	assert.Equal(t, string(expected), buf.String())
}
