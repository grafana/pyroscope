package tree

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"io"

	"github.com/pyroscope-io/pyroscope/pkg/storage/dict"
	"github.com/pyroscope-io/pyroscope/pkg/util/varint"
)

type parentNode struct {
	node   int
	parent *parentNode
}

func DeserializeV1(d *dict.Dict, br *bufio.Reader) (*Tree, error) {
	t := New()
	parents := []*parentNode{{0, nil}}
	j := 0

	var nameBuf bytes.Buffer
	for len(parents) > 0 {
		j++
		parent := parents[0]
		parents = parents[1:]

		labelLen, err := varint.Read(br)
		labelLinkBuf := make([]byte, labelLen) // TODO: there are better ways to do this?
		_, err = io.ReadAtLeast(br, labelLinkBuf, int(labelLen))
		if err != nil {
			return nil, err
		}

		nameBuf.Reset()
		if !d.GetValue(labelLinkBuf, &nameBuf) {
			// these strings has to be at least slightly different, hence base64 Addon
			nameBuf.Reset()
			nameBuf.WriteString("label not found " + base64.URLEncoding.EncodeToString(labelLinkBuf))
		}
		if cap(t.nodes)-len(t.nodes) == 0 {
			t.grow(1)
		}
		tn, z := t.insert(t.at(parent.node), nameBuf.Bytes())
		tn.Self, err = varint.Read(br)
		tn.Total = tn.Self
		if err != nil {
			return nil, err
		}

		pn := parent
		for pn != nil {
			t.at(pn.node).Total += tn.Self
			pn = pn.parent
		}

		childrenLen, err := varint.Read(br)
		if err != nil {
			return nil, err
		}

		for i := uint64(0); i < childrenLen; i++ {
			parents = append([]*parentNode{{z, parent}}, parents...)
		}
	}

	firstChildIdx := t.nodes[0].ChildrenNodes[0]
	t.nodes[0] = t.nodes[firstChildIdx]
	return t, nil
}
