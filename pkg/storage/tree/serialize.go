package tree

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"io"

	"github.com/petethepig/pyroscope/pkg/storage/dict"
	"github.com/petethepig/pyroscope/pkg/util/varint"
	log "github.com/sirupsen/logrus"
)

func (t *Tree) Serialize(d *dict.Dict, w io.Writer) error {
	nodes := []*treeNode{t.root}
	// TODO: pass config value
	minVal := t.minValue(1024)
	j := 0

	for len(nodes) > 0 {
		j++
		tn := nodes[0]
		nodes = nodes[1:]

		labelLink := d.Put(tn.name)
		_, err := varint.Write(w, uint64(len(labelLink)))
		if err != nil {
			return err
		}
		_, err = w.Write(labelLink)
		if err != nil {
			return err
		}

		val := tn.self
		_, err = varint.Write(w, uint64(val))
		if err != nil {
			return err
		}
		var cnl = uint64(0)
		if tn.cum > minVal {
			cnl = uint64(len(tn.childrenNodes))
			nodes = append(tn.childrenNodes, nodes...)
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

		labelLen, err := binary.ReadUvarint(br)
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
			// these strings has to be at least slightly different, hence base64 addition
			nameBuf = []byte("label not found " + base64.URLEncoding.EncodeToString(labelLinkBuf))
		}
		tn := parent.node.insert(nameBuf)

		tn.self, err = binary.ReadUvarint(br)
		tn.cum = tn.self
		if err != nil {
			return nil, err
		}

		pn := parent
		for pn != nil {
			pn.node.cum += tn.self
			pn = pn.parent
		}

		childrenLen, err := binary.ReadUvarint(br)
		if err != nil {
			return nil, err
		}

		for i := uint64(0); i < childrenLen; i++ {
			parents = append([]*parentNode{{tn, parent}}, parents...)
		}
	}

	log.Debug("deserialize node count", j)

	t.root = t.root.childrenNodes[0]

	return t, nil
}

func (t *Tree) Bytes(d *dict.Dict) []byte {
	b := bytes.Buffer{}
	t.Serialize(d, &b)
	return b.Bytes()
}
func FromBytes(d *dict.Dict, p []byte) *Tree {
	t, _ := Deserialize(d, bytes.NewReader(p))
	return t
}
