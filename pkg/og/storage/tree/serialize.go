package tree

import (
	"bufio"
	"io"

	"github.com/grafana/pyroscope/pkg/og/util/varint"
)

type parentNode struct {
	node   *treeNode
	parent *parentNode
}

// used in the cloud
func DeserializeNoDict(r io.Reader) (*Tree, error) {
	t := New()
	br := bufio.NewReader(r) // TODO if it's already a bytereader skip

	parents := []*parentNode{{t.root, nil}}
	j := 0

	for len(parents) > 0 {
		j++
		parent := parents[0]
		parents = parents[1:]

		nameLen, err := varint.Read(br)
		// if err == io.EOF {
		// 	return t, nil
		// }
		nameBuf := make([]byte, nameLen) // TODO: there are better ways to do this?
		_, err = io.ReadAtLeast(br, nameBuf, int(nameLen))
		if err != nil {
			return nil, err
		}
		tn := parent.node.insert(nameBuf)

		tn.Self, err = varint.Read(br)
		tn.Total = tn.Self
		if err != nil {
			return nil, err
		}

		pn := parent
		for pn != nil {
			pn.node.Total += tn.Self
			pn = pn.parent
		}

		childrenLen, err := varint.Read(br)
		if err != nil {
			return nil, err
		}

		for i := uint64(0); i < childrenLen; i++ {
			parents = append([]*parentNode{{tn, parent}}, parents...)
		}
	}

	t.root = t.root.ChildrenNodes[0]

	return t, nil
}
