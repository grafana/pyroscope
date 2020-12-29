package segment

import (
	"bufio"
	"bytes"
	"io"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/util/serialization"
	"github.com/pyroscope-io/pyroscope/pkg/util/varint"
)

// serialization format version. it's not very useful right now, but it will be in the future
const currentVersion = 1

func (s *Segment) populateFromMetadata(metadata map[string]interface{}) {
	if v, ok := metadata["sampleRate"]; ok {
		s.sampleRate = int(v.(float64))
	}
	if v, ok := metadata["spyName"]; ok {
		s.spyName = v.(string)
	}
}

func (s *Segment) generateMetadata() map[string]interface{} {
	return map[string]interface{}{
		"sampleRate": s.sampleRate,
		"spyName":    s.spyName,
	}
}

func (s *Segment) Serialize(w io.Writer) error {
	s.m.RLock()
	defer s.m.RUnlock()

	varint.Write(w, currentVersion)

	serialization.WriteMetadata(w, s.generateMetadata())

	nodes := []*streeNode{s.root}
	for len(nodes) > 0 {
		n := nodes[0]
		varint.Write(w, uint64(n.depth))
		varint.Write(w, uint64(n.time.Unix()))
		varint.Write(w, n.samples)
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

	// reads serialization format version, see comment at the top
	_, err := varint.Read(br)
	if err != nil {
		return nil, err
	}

	metadata, err := serialization.ReadMetadata(br)
	if err != nil {
		return nil, err
	}
	s.populateFromMetadata(metadata)

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
		samplesVal, err := varint.Read(br)
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
		node.samples = samplesVal
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
	// TODO: handle error
	t, _ := Deserialize(resolution, multiplier, bytes.NewReader(p))
	return t
}
