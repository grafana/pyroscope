package dict

import (
	"bufio"
	"bytes"
	"io"

	"github.com/petethepig/pyroscope/pkg/util/varint"
)

func (t *Dict) Serialize(w io.Writer) error {
	nodes := []*trieNode{t.root}
	for len(nodes) > 0 {
		tn := nodes[0]
		nodes = nodes[1:]

		label := tn.label
		_, err := varint.Write(w, uint64(len(label)))
		if err != nil {
			return err
		}
		_, err = w.Write(label)
		if err != nil {
			return err
		}

		_, err = varint.Write(w, uint64(len(tn.children)))
		if err != nil {
			return err
		}

		nodes = append(tn.children, nodes...)
	}
	return nil
}

func Deserialize(r io.Reader) (*Dict, error) {
	t := New()
	br := bufio.NewReader(r) // TODO if it's already a bytereader skip

	parents := []*trieNode{t.root}
	for len(parents) > 0 {
		parent := parents[0]
		parents = parents[1:]

		nameLen, err := varint.Read(br)
		nameBuf := make([]byte, nameLen) // TODO: maybe there are better ways to do this?
		_, err = io.ReadAtLeast(br, nameBuf, int(nameLen))
		if err != nil {
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

func (t *Dict) Bytes() []byte {
	b := bytes.Buffer{}
	t.Serialize(&b)
	return b.Bytes()
}

func FromBytes(p []byte) *Dict {
	t, _ := Deserialize(bytes.NewReader(p))
	return t
}
