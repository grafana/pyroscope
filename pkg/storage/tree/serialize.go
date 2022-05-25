package tree

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"io"

	"github.com/pyroscope-io/pyroscope/pkg/storage/dict"
	"github.com/pyroscope-io/pyroscope/pkg/util/varint"
)

// serialization format version. it's not very useful right now, but it will be in the future
const currentVersion = 1

func (t *Tree) Serialize(d *dict.Dict, maxNodes int, w io.Writer) error {
	t.RLock()
	defer t.RUnlock()

	varint.Write(w, currentVersion)

	nodes := []*treeNode{t.root}
	minVal := t.minValue(maxNodes)
	j := 0

	for len(nodes) > 0 {
		j++
		tn := nodes[0]
		nodes = nodes[1:]

		labelLink := d.Put([]byte(tn.Name))
		_, err := varint.Write(w, uint64(len(labelLink)))
		if err != nil {
			return err
		}
		_, err = w.Write(labelLink)
		if err != nil {
			return err
		}

		val := tn.Self
		_, err = varint.Write(w, uint64(val))
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

func (t *Tree) SerializeTruncate(d *dict.Dict, maxNodes int, w io.Writer) error {
	t.Lock()
	defer t.Unlock()
	vw := varint.NewWriter()
	var err error
	if _, err = vw.Write(w, currentVersion); err != nil {
		return err
	}

	minVal := t.minValue(maxNodes)
	nodes := make([]*treeNode, 1, 128)
	nodes[0] = t.root
	for len(nodes) > 0 {
		tn := nodes[0]
		nodes = nodes[1:]

		labelKey := d.Put([]byte(tn.Name))
		if _, err = vw.Write(w, uint64(len(labelKey))); err != nil {
			return err
		}
		if _, err = w.Write(labelKey); err != nil {
			return err
		}
		val := tn.Self
		if _, err = vw.Write(w, val); err != nil {
			return err
		}

		cNodes := tn.ChildrenNodes
		tn.ChildrenNodes = tn.ChildrenNodes[:0]
		for _, cn := range cNodes {
			if cn.Total >= minVal {
				tn.ChildrenNodes = append(tn.ChildrenNodes, cn)
			}
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

func (t *Tree) SerializeNoDict(maxNodes int, w io.Writer) error {
	t.RLock()
	defer t.RUnlock()

	nodes := []*treeNode{t.root}
	minVal := t.minValue(maxNodes)
	j := 0

	for len(nodes) > 0 {
		j++
		tn := nodes[0]
		nodes = nodes[1:]

		_, err := varint.Write(w, uint64(len(tn.Name)))
		if err != nil {
			return err
		}
		_, err = w.Write(tn.Name)
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

type parentNode struct {
	node   *treeNode
	parent *parentNode
}

func Deserialize(d *dict.Dict, r io.Reader) (*Tree, error) {
	t := New()
	br := bufio.NewReader(r) // TODO if it's already a bytereader skip

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

func (t *Tree) Bytes(d *dict.Dict, maxNodes int) ([]byte, error) {
	b := bytes.Buffer{}
	if err := t.Serialize(d, maxNodes, &b); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func FromBytes(d *dict.Dict, p []byte) (*Tree, error) {
	return Deserialize(d, bytes.NewReader(p))
}

func (t *Tree) SerializeNoDictNoLimit(w io.Writer) error {
	t.RLock()
	defer t.RUnlock()

	nodes := []*treeNode{t.root}
	j := 0

	for len(nodes) > 0 {
		j++
		tn := nodes[0]
		nodes = nodes[1:]

		_, err := varint.Write(w, uint64(len(tn.Name)))
		if err != nil {
			return err
		}
		_, err = w.Write(tn.Name)
		if err != nil {
			return err
		}

		val := tn.Self
		_, err = varint.Write(w, val)
		if err != nil {
			return err
		}
		cnl := uint64(0)
		cnl = uint64(len(tn.ChildrenNodes))
		nodes = append(tn.ChildrenNodes, nodes...)
		_, err = varint.Write(w, cnl)
		if err != nil {
			return err
		}
	}
	return nil
}

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

		val := tn.Self
		cNodes := tn.ChildrenNodes
		tn.ChildrenNodes = tn.ChildrenNodes[:0]
		for _, cn := range cNodes {
			if cn.Total >= minVal {
				tn.ChildrenNodes = append(tn.ChildrenNodes, cn)
			} else {
				// Truncated children accounted as parent self.
				val += cn.Total
			}
		}
		if _, err = vw.Write(w, val); err != nil {
			return err
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
