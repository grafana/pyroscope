package query_plan

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/iter"
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
	printPlan(&buf, "", p, true)
	// Ensure that the plan has not been modified
	// during traversal performed by printPlan.
	assert.Equal(t, Build(blocks, 2, 3), p)

	expected, err := os.ReadFile("testdata/plan.txt")
	require.NoError(t, err)
	assert.Equal(t, string(expected), buf.String())

	// Root node (sub-)plan must be identical to the original plan.
	buf.Reset()
	printPlan(&buf, "", p.Root().Plan(), true)
	assert.Equal(t, string(expected), buf.String())
}

func Test_Plan_propagation(t *testing.T) {
	blocks := []*metastorev1.BlockMeta{
		{Id: "01J2JY1K5J4T2WNDV05CHVFCA9"},
		{Id: "01J2JY21VVYYV4PMDGK4TVMZ6H"},
		{Id: "01J2JY2GF83EF0QMW94T19MXHN"},
		{Id: "01J2JY45S90MWF6ZER08BFGPGP"},
		{Id: "01J2JY5JR0C9V64EP32RPH61E7"},
		{Id: "01J2JY61BG7QBPNK54EY8N0T6K"},
		{Id: "01J2JZ0A7MPZMR0R745HAZD1S9"},
		{Id: "01J2JZ0RY9WCA01S322EG201R8"},
	}

	var buf bytes.Buffer
	expected := `merge
read [id:"01J2JY1K5J4T2WNDV05CHVFCA9" id:"01J2JY21VVYYV4PMDGK4TVMZ6H"]
read [id:"01J2JY2GF83EF0QMW94T19MXHN" id:"01J2JY45S90MWF6ZER08BFGPGP"]
read [id:"01J2JY5JR0C9V64EP32RPH61E7" id:"01J2JY61BG7QBPNK54EY8N0T6K"]
read [id:"01J2JZ0A7MPZMR0R745HAZD1S9" id:"01J2JZ0RY9WCA01S322EG201R8"]
`

	p := Build(blocks, 2, 5).Proto()
	n := []*queryv1.QueryPlan{p}
	var x *QueryPlan
	for len(n) > 0 {
		x, n = Open(n[0]), n[1:]

		switch r := x.Root(); r.Type {
		case NodeMerge:
			_, _ = fmt.Fprintln(&buf, "merge")
			c := r.Children()
			for c.Next() {
				n = append(n, c.At().Plan().Proto())
			}

		case NodeRead:
			_, _ = fmt.Fprintln(&buf, "read", iter.MustSlice(r.Blocks()))

		default:
			panic("query plan: unknown node type")
		}
	}

	require.Equal(t, expected, buf.String())
}

func Test_Plan_skip_top_merge(t *testing.T) {
	blocks := []*metastorev1.BlockMeta{
		{Id: "01J2JY1K5J4T2WNDV05CHVFCA9"},
		{Id: "01J2JY21VVYYV4PMDGK4TVMZ6H"},
	}

	var buf bytes.Buffer
	expected := `[id:"01J2JY1K5J4T2WNDV05CHVFCA9" id:"01J2JY21VVYYV4PMDGK4TVMZ6H"]`

	p := Build(blocks, 2, 5).Proto()
	n := []*queryv1.QueryPlan{p}
	var x *QueryPlan
	for len(n) > 0 {
		x, n = Open(n[0]), n[1:]

		switch r := x.Root(); r.Type {
		case NodeMerge:
			_, _ = fmt.Fprintln(&buf, "merge")
			c := r.Children()
			for c.Next() {
				n = append(n, c.At().Plan().Proto())
			}

		case NodeRead:
			_, _ = fmt.Fprint(&buf, iter.MustSlice(r.Blocks()))

		default:
			panic("query plan: unknown node type")
		}
	}

	require.Equal(t, expected, buf.String())
}
