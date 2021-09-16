package dict

import (
	"bufio"
	"bytes"
	"io"

	"github.com/pyroscope-io/pyroscope/pkg/util/varint"
)

// serialization format version. it's not very useful right now, but it will be in the future
const currentVersion = 1

func (t *Dict) Serialize(w io.Writer) error {
	vw := varint.NewWriter()
	vw.Write(w, currentVersion)
	vw.Write(w, uint64(len(t.keys.data)))
	_, err := w.Write(t.keys.data)
	if err != nil {
		return err
	}

	nodes := []*trieNode{t.root}
	for len(nodes) > 0 {
		tn := nodes[0]
		nodes = nodes[1:]

		label := tn.label
		if _, err = varint.Write(w, uint64(len(label))); err != nil {
			return err
		}
		if _, err = w.Write(label); err != nil {
			return err
		}

		if _, err = varint.Write(w, uint64(len(tn.children))); err != nil {
			return err
		}

		nodes = append(tn.children, nodes...)
	}
	return nil
}

func Deserialize(r io.Reader) (*Dict, error) {
	t := New()
	br := bufio.NewReader(r) // TODO if it's already a bytereader skip

	// reads serialization format version, see comment at the top
	_, err := varint.Read(br)
	if err != nil {
		return nil, err
	}
	keysLen, err := varint.Read(br)
	if err != nil {
		return nil, err
	}
	t.keys.data = make([]byte, int(keysLen))
	if _, err = io.ReadAtLeast(br, t.keys.data, len(t.keys.data)); err != nil {
		return nil, err
	}

	parents := []*trieNode{t.root}
	for len(parents) > 0 {
		parent := parents[0]
		parents = parents[1:]

		nameLen, err := varint.Read(br)
		nameBuf := make([]byte, nameLen) // TODO: maybe there are better ways to do this?
		if _, err = io.ReadAtLeast(br, nameBuf, int(nameLen)); err != nil {
			return nil, err
		}
		tn := newTrieNode(nameBuf)
		parent.insert(tn)

		childrenLen, err := varint.Read(br)
		if err != nil {
			return nil, err
		}

		for i := uint64(0); i < childrenLen; i++ {
			parents = append([]*trieNode{tn}, parents...)
		}
	}

	t.root = t.root.children[0]

	return t, nil
}

func (t *Dict) Bytes() ([]byte, error) {
	var b bytes.Buffer
	if err := t.Serialize(&b); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func FromBytes(p []byte) (*Dict, error) {
	return Deserialize(bytes.NewReader(p))
}
