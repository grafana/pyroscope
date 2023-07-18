package tree

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"io"

	"github.com/grafana/pyroscope/pkg/og/storage/dict"
	"github.com/grafana/pyroscope/pkg/og/util/varint"
)

// serialization format version. it's not very useful right now, but it will be in the future
const currentVersion = 1

var lostDuringSerializationName = []byte("other")

// warning: this function modifies the tree
func (t *Tree) SerializeTruncate(d *dict.Dict, maxNodes int, w io.Writer) error {
	t.Lock()
	defer t.Unlock()
	vw := varint.NewWriter()
	var err error
	if _, err = vw.Write(w, currentVersion); err != nil {
		return err
	}

	var b bytes.Buffer // Temporary buffer for dictionary keys.
	minVal := t.minValue(maxNodes)
	nodes := make([]*treeNode, 1, 128)
	nodes[0] = t.root
	for len(nodes) > 0 {
		tn := nodes[0]
		nodes = nodes[1:]

		b.Reset()
		d.PutValue([]byte(tn.Name), &b)
		if _, err = vw.Write(w, uint64(b.Len())); err != nil {
			return err
		}
		if _, err = w.Write(b.Bytes()); err != nil {
			return err
		}
		if _, err = vw.Write(w, tn.Self); err != nil {
			return err
		}

		cNodes := tn.ChildrenNodes
		tn.ChildrenNodes = tn.ChildrenNodes[:0]

		other := uint64(0)
		for _, cn := range cNodes {
			isOtherNode := bytes.Equal(cn.Name, lostDuringSerializationName)
			if cn.Total >= minVal || isOtherNode {
				tn.ChildrenNodes = append(tn.ChildrenNodes, cn)
			} else {
				// Truncated children accounted as parent self.
				other += cn.Total
			}
		}

		if other > 0 {
			otherNode := tn.insert(lostDuringSerializationName)
			otherNode.Self += other
			otherNode.Total += other
		}

		if len(tn.ChildrenNodes) > 0 {
			nodes = append(tn.ChildrenNodes, nodes...)
		} else {
			tn.ChildrenNodes = nil // Just to make it eligible for GC.
		}
		if _, err = vw.Write(w, uint64(len(tn.ChildrenNodes))); err != nil {
			return err
		}
	}
	return nil
}

type parentNode struct {
	node   *treeNode
	parent *parentNode
}

func Deserialize(d *dict.Dict, r io.Reader) (*Tree, error) {
	t := New()

	type reader interface {
		io.ByteReader
		io.Reader
	}
	var br reader
	switch x := r.(type) {
	case *bytes.Buffer:
		br = x
	case *bytes.Reader:
		br = x
	case *bufio.Reader:
		br = x
	default:
		br = bufio.NewReader(r)
	}

	// reads serialization format version, see comment at the top
	_, err := varint.Read(br)
	if err != nil {
		return nil, err
	}

	parents := []*parentNode{{t.root, nil}}
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
		tn := parent.node.insert(nameBuf.Bytes())
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

// used in the cloud
// warning: this function modifies the tree
func (t *Tree) SerializeTruncateNoDict(maxNodes int, w io.Writer) error {
	t.Lock()
	defer t.Unlock()
	vw := varint.NewWriter()
	var err error
	minVal := t.minValue(maxNodes)
	nodes := make([]*treeNode, 1, 1024)
	nodes[0] = t.root
	for len(nodes) > 0 {
		tn := nodes[0]
		nodes = nodes[1:]
		if _, err = vw.Write(w, uint64(len(tn.Name))); err != nil {
			return err
		}
		if _, err = w.Write(tn.Name); err != nil {
			return err
		}

		if _, err = vw.Write(w, tn.Self); err != nil {
			return err
		}
		cNodes := tn.ChildrenNodes
		tn.ChildrenNodes = tn.ChildrenNodes[:0]

		other := uint64(0)
		for _, cn := range cNodes {
			isOtherNode := bytes.Equal(cn.Name, lostDuringSerializationName)
			if cn.Total >= minVal || isOtherNode {
				tn.ChildrenNodes = append(tn.ChildrenNodes, cn)
			} else {
				// Truncated children accounted as parent self.
				other += cn.Total
			}
		}

		if other > 0 {
			otherNode := tn.insert(lostDuringSerializationName)
			otherNode.Self += other
			otherNode.Total += other
		}

		if len(tn.ChildrenNodes) > 0 {
			nodes = append(tn.ChildrenNodes, nodes...)
		} else {
			tn.ChildrenNodes = nil // Just to make it eligible for GC.
		}
		if _, err = vw.Write(w, uint64(len(tn.ChildrenNodes))); err != nil {
			return err
		}
	}
	return nil
}
