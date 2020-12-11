package tree

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"io"

	"github.com/petethepig/pyroscope/pkg/storage/dict"
	"github.com/petethepig/pyroscope/pkg/util/varint"
	log "github.com/sirupsen/logrus"
)

func (t *Tree) Serialize(d *dict.Dict, maxNodes int, w io.Writer) error {
	t.m.RLock()
	defer t.m.RUnlock()

	nodes := []*treeNode{t.root}
	// TODO: pass config value
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
		var cnl = uint64(0)
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

	parents := []*parentNode{{t.root, nil}}
	j := 0

	for len(parents) > 0 {
		j++
		parent := parents[0]
		parents = parents[1:]

		labelLen, err := varint.Read(br)
		// if err == io.EOF {
		// 	return t, nil
		// }
		labelLinkBuf := make([]byte, labelLen) // TODO: there are better ways to do this?
		_, err = io.ReadAtLeast(br, labelLinkBuf, int(labelLen))
		if err != nil {
			return nil, err
		}
		nameBuf, ok := d.Get(labelLinkBuf)
		if !ok {
			// these strings has to be at least slightly different, hence base64 Addon
			nameBuf = []byte("label not found " + base64.URLEncoding.EncodeToString(labelLinkBuf))
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

	log.Debug("deserialize node count", j)

	t.root = t.root.ChildrenNodes[0]

	return t, nil
}

func (t *Tree) Bytes(d *dict.Dict, maxNodes int) []byte {
	b := bytes.Buffer{}
	t.Serialize(d, maxNodes, &b)
	return b.Bytes()
}
func FromBytes(d *dict.Dict, p []byte) *Tree {
	t, _ := Deserialize(d, bytes.NewReader(p))
	return t
}
