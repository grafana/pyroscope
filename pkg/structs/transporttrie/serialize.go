package transporttrie

import (
	"bufio"
	"bytes"
	"errors"
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

type offset struct {
	descCount int
	suffixLen int
}

// IterateRaw iterates through the serialized trie and calls cb function for
// every leaf. k references bytes from buf, therefore it must not be modified
// or used outside of cb, a copy of k should be used instead.
func IterateRaw(r io.Reader, buf []byte, cb func(k []byte, v int)) error {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}

	b := bytes.NewBuffer(buf)
	var offsets []offset
	var copied int64
	for {
		nameLen, err := varint.Read(br)
		switch {
		case err == nil:
		case errors.Is(err, io.EOF):
			return nil
		default:
			return err
		}
		if nameLen != 0 {
			copied, err = io.CopyN(b, br, int64(nameLen))
			if err != nil {
				return err
			}
		}
		value, err := varint.Read(br)
		if err != nil {
			return err
		}
		descCount, err := varint.Read(br)
		if err != nil {
			return err
		}

		// It may be a node or a leaf. Regardless, if it has
		// a value, there was a corresponding signature.
		if value > 0 {
			cb(b.Bytes(), int(value))
		}

		if descCount != 0 {
			// A node. Add node suffix and save offset.
			offsets = append(offsets, offset{
				descCount: int(descCount),
				suffixLen: int(copied),
			})
			continue
		}

		// A leaf. Cut the current label.
		b.Truncate(b.Len() - int(copied))
		// Cut parent suffix, if it has no more
		// descendants, and it is not the root.
		i := len(offsets) - 1
		if i < 0 {
			continue
		}
		offsets[i].descCount--
		for ; i > 0; i-- {
			if offsets[i].descCount != 0 {
				break
			}
			// No descending nodes left.
			// Cut suffix and remove the offset.
			b.Truncate(b.Len() - offsets[i].suffixLen)
			offsets = offsets[:i]
			// Decrease parent counter, if applicable.
			if p := len(offsets) - 1; p > 0 {
				offsets[p].descCount--
			}
		}
	}
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
