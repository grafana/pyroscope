package tree

import (
	"fmt"
	"io/ioutil"

	"github.com/golang/protobuf/jsonpb"
	"google.golang.org/protobuf/proto"
)

func (t *Tree) Pprof() []byte {
	var pprof Profile = Profile{
		StringTable: []string{""},
	}
	var i int64 = 0

	t.Iterate(func(k []byte, v uint64) {
		if v > 0 {
			i++
			pprof.StringTable = append(pprof.StringTable, string(k))
			label := Label{Key: i, Str: i}
			pprof.Sample = append(pprof.Sample, &Sample{Label: []*Label{&label}, Value: []int64{int64(v)}})
		}
	})
	out, err := proto.Marshal(&pprof)
	if err == nil {
		m := jsonpb.Marshaler{}
		result, _ := m.MarshalToString(&pprof)
		ioutil.WriteFile("./pprof.json", []byte(result), 0600)
		fmt.Println(result)
		return out
	}
	panic("error")

}

func (t *Tree) Iterate2(cb func(key []byte, val uint64)) {
	nodes := []*treeNode{t.root}
	prefixes := make([][]byte, 1)
	prefixes[0] = make([]byte, 0)
	for len(nodes) > 0 { // bfs
		node := nodes[0]
		nodes = nodes[1:]

		prefix := prefixes[0]
		prefixes = prefixes[1:]

		label := append(prefix, semicolon) // byte(';'),
		l := node.Name
		label = append(label, l...) // byte(';'),

		cb(label, node.Self)

		nodes = append(node.ChildrenNodes, nodes...)
		for i := 0; i < len(node.ChildrenNodes); i++ {
			prefixes = prependBytes(prefixes, label)
		}
	}
}
