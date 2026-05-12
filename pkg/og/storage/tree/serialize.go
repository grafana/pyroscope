package tree

import (
	"bufio"
	"fmt"
	"io"

	"github.com/grafana/pyroscope/v2/pkg/og/util/varint"
)

type parentNode struct {
	node   *treeNode
	parent *parentNode
}

// used in the cloud
func DeserializeNoDict(r io.Reader, maxNameLen, maxChildren int) (*Tree, error) {
	t := New()
	br := bufio.NewReader(r) // TODO if it's already a bytereader skip

	parents := []*parentNode{{t.root, nil}}
	j := 0

	for len(parents) > 0 {
		j++
		parent := parents[0]
		parents = parents[1:]

		nameLen, err := varint.Read(br)
		if err != nil {
			return nil, err
		}
		if maxNameLen > 0 && nameLen > uint64(maxNameLen) {
			return nil, fmt.Errorf("tree node name length %d exceeds maximum %d", nameLen, maxNameLen)
		}
		nameBuf := make([]byte, nameLen)
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
		if maxChildren > 0 && childrenLen > uint64(maxChildren) {
			return nil, fmt.Errorf("tree node children count %d exceeds maximum %d", childrenLen, maxChildren)
		}

		for i := uint64(0); i < childrenLen; i++ {
			parents = append([]*parentNode{{tn, parent}}, parents...)
		}
	}

	t.root = t.root.ChildrenNodes[0]

	return t, nil
}
