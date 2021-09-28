package tree

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"io"

	"github.com/pyroscope-io/pyroscope/pkg/storage/dict"
	"github.com/pyroscope-io/pyroscope/pkg/util/varint"
)

// V1 format support is preserved for backward compatibility and should not
// be considered as a candidate for improvements.
//
// Serialization and deserialization w/o a dictionary is supported only for V1:
//  - serialization is only used as 'pyroscope convert tree' output format.
//  - deserialization is used for 'binary/octet-stream+tree' ingestion input format.

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
		p := t.insert(parent.node, nameBuf.Bytes())
		tn := t.at(p)
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
			parents = append([]*parentNode{{p, parent}}, parents...)
		}
	}

	firstChildIdx := t.nodes[0].ChildrenNodes[0]
	t.nodes[0] = t.nodes[firstChildIdx]
	return t, nil
}

func DeserializeV1NoDict(r io.Reader) (*Tree, error) {
	t := New()
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}

	parents := []*parentNode{{0, nil}}
	j := 0

	for len(parents) > 0 {
		j++
		parent := parents[0]
		parents = parents[1:]

		labelLen, err := varint.Read(br)
		label := make([]byte, labelLen)
		_, err = io.ReadAtLeast(br, label, int(labelLen))
		if err != nil {
			return nil, err
		}
		p := t.insert(parent.node, label)
		tn := t.at(p)
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
			parents = append([]*parentNode{{p, parent}}, parents...)
		}
	}

	firstChildIdx := t.nodes[0].ChildrenNodes[0]
	t.nodes[0] = t.nodes[firstChildIdx]
	return t, nil
}

func (t *Tree) SerializeV1NoDict(maxNodes int, w io.Writer) error {
	t.Lock()
	defer t.Unlock()

	nodes := []int{0}
	minVal := t.minValue(maxNodes)
	j := 0

	for len(nodes) > 0 {
		j++
		tn := t.at(nodes[0])
		nodes = nodes[1:]

		label := t.loadLabel(tn.labelPosition)
		_, err := varint.Write(w, uint64(len(label)))
		if err != nil {
			return err
		}
		_, err = w.Write(label)
		if err != nil {
			return err
		}

		val := tn.Self
		_, err = varint.Write(w, val)
		if err != nil {
			return err
		}
		cnl := uint64(0)
		if tn.Total > minVal {
			cnl = uint64(len(tn.ChildrenNodes))
			nodes = append(tn.ChildrenNodes, nodes...)
		}
		_, err = varint.Write(w, cnl)
		if err != nil {
			return err
		}
	}

	return nil
}
