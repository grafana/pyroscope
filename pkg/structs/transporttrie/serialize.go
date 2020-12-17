package transporttrie

import (
	"bufio"
	"bytes"
	"io"

	"github.com/pyroscope-io/pyroscope/pkg/util/varint"
)

func (t *Trie) Serialize(w io.Writer) error {
	nodes := []*trieNode{t.root}
	for len(nodes) > 0 {
		tn := nodes[0]
		nodes = nodes[1:]

		name := tn.name
		_, err := varint.Write(w, uint64(len(name)))
		if err != nil {
			return err
		}
		_, err = w.Write(name)
		if err != nil {
			return err
		}

		val := tn.value
		if t.Divider != 1 || t.Multiplier != 1 {
			val = val * uint64(t.Multiplier) / uint64(t.Divider)
		}
		_, err = varint.Write(w, uint64(val))
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

func Deserialize(r io.Reader) (*Trie, error) {
	t := New()
	br := bufio.NewReader(r) // TODO if it's already a bytereader skip

	parents := []*trieNode{t.root}
	for len(parents) > 0 {
		parent := parents[0]
		parents = parents[1:]

		nameLen, err := varint.Read(br)
		// if err == io.EOF {
		// 	return t, nil
		// }
		nameBuf := make([]byte, nameLen) // TODO: there are better ways to do this?
		_, err = io.ReadAtLeast(br, nameBuf, int(nameLen))
		// log.Debug(n, len(parents))
		// log.Debugf("%d", nameLen, string(nameBuf), n)
		if err != nil {
			return nil, err
		}
		tn := newTrieNode(nameBuf)
		// TODO: insert into parent
		parent.insert(tn)

		tn.value, err = varint.Read(br)
		if err != nil {
			return nil, err
		}

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

func (t *Trie) Bytes() []byte {
	b := bytes.Buffer{}
	t.Serialize(&b)
	return b.Bytes()
}

func FromBytes(p []byte) *Trie {
	t, _ := Deserialize(bytes.NewReader(p))
	return t
}
