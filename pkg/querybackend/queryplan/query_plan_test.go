package queryplan

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

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
	printPlan(&buf, "", p.Root, true)
	// Ensure that the plan has not been modified
	// during traversal performed by printPlan.
	assert.Equal(t, Build(blocks, 2, 3), p)

	expected, err := os.ReadFile("testdata/plan.txt")
	require.NoError(t, err)
	assert.Equal(t, string(expected), buf.String())
}
