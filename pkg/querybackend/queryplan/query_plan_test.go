package queryplan

import (
	"testing"

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
	switch r := p.Root(); r.Type {
	case NodeMerge:
		t.Logf("merge: %+v", r.n)
		c := r.Children()
		for c.Next() {
			n := c.At()
			t.Logf(" - child %+v", n.n)
			t.Logf("         %+v", n.Plan())
		}

	case NodeRead:
		t.Logf("read: %+v", r.n)
		t.Log(r.Blocks())

	default:
		panic("unknown type")
	}
}
