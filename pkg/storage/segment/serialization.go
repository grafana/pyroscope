package segment

import (
	"bufio"
	"bytes"
	"io"
	"time"

	"github.com/petethepig/pyroscope/pkg/util/varint"
)

func (s *Segment) Serialize(w io.Writer) error {
	nodes := []*streeNode{s.root}
	for len(nodes) > 0 {
		n := nodes[0]
		varint.Write(w, uint64(n.depth))
		varint.Write(w, uint64(n.time.Unix()))
		p := uint64(0)
		if n.present {
			p = 1
		}
		varint.Write(w, p)
		nodes = nodes[1:]

		// depth
		// time
		// keyInChunks
		// children
		l := 0
		r := []*streeNode{}
		for _, v := range n.children {
			if v != nil {
				l++
				r = append(r, v)
			}
		}

		varint.Write(w, uint64(l))
		nodes = append(r, nodes...)
	}
	return nil
}

func Deserialize(resolution time.Duration, multiplier int, r io.Reader) (*Segment, error) {
	s := New(resolution, multiplier)
	br := bufio.NewReader(r) // TODO if it's already a bytereader skip

	parents := []*streeNode{nil}
	for len(parents) > 0 {
		depth, err := varint.Read(br)
		if err != nil {
			return nil, err
		}
		timeVal, err := varint.Read(br)
		if err != nil {
			return nil, err
		}
		presentVal, err := varint.Read(br)
		if err != nil {
			return nil, err
		}
		node := newNode(time.Unix(int64(timeVal), 0), int(depth), s.multiplier)
		if presentVal == 1 {
			node.present = true
		}
		if s.root == nil {
			s.root = node
		}

		parent := parents[0]
		parents = parents[1:]
		if parent != nil {
			parent.replace(node)
		}
		childrenLen, err := varint.Read(br)
		if err != nil {
			return nil, err
		}

		r := []*streeNode{}
		for i := 0; i < int(childrenLen); i++ {
			r = append(r, node)
		}
		parents = append(r, parents...)
	}

	return s, nil
}

func (t *Segment) Bytes() []byte {
	b := bytes.Buffer{}
	t.Serialize(&b)
	return b.Bytes()
}

func FromBytes(resolution time.Duration, multiplier int, p []byte) *Segment {
	t, _ := Deserialize(resolution, multiplier, bytes.NewReader(p))
	return t
}
