package segment

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/util/serialization"
	"github.com/pyroscope-io/pyroscope/pkg/util/varint"
)

// serialization format version. it's not very useful right now, but it will be in the future
const currentVersion = 2

func (s *Segment) populateFromMetadata(metadata map[string]interface{}) {
	if v, ok := metadata["sampleRate"]; ok {
		s.sampleRate = uint32(v.(float64))
	}
	if v, ok := metadata["spyName"]; ok {
		s.spyName = v.(string)
	}
	if v, ok := metadata["units"]; ok {
		s.units = v.(string)
	}
	if v, ok := metadata["aggregationType"]; ok {
		s.aggregationType = v.(string)
	}
}

func (s *Segment) generateMetadata() map[string]interface{} {
	return map[string]interface{}{
		"sampleRate":      s.sampleRate,
		"spyName":         s.spyName,
		"units":           s.units,
		"aggregationType": s.aggregationType,
	}
}

func (s *Segment) Serialize(w io.Writer) error {
	s.m.RLock()
	defer s.m.RUnlock()

	vw := varint.NewWriter()

	vw.Write(w, currentVersion)

	serialization.WriteMetadata(w, s.generateMetadata())

	if s.root == nil {
		return nil
	}

	nodes := []*streeNode{s.root}
	for len(nodes) > 0 {
		n := nodes[0]
		vw.Write(w, uint64(n.depth))
		vw.Write(w, uint64(n.time.Unix()))
		vw.Write(w, n.samples)
		vw.Write(w, n.writes)
		p := uint64(0)
		if n.present {
			p = 1
		}
		vw.Write(w, p)
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

		vw.Write(w, uint64(l))
		nodes = append(r, nodes...)
	}
	return nil
}

var errMaxDepth = errors.New("depth is too high")

func Deserialize(r io.Reader) (*Segment, error) {
	s := New()
	br := bufio.NewReader(r) // TODO if it's already a bytereader skip

	// reads serialization format version, see comment at the top
	version, err := varint.Read(br)
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
		if int(depth) >= len(durations) {
			return nil, errMaxDepth
		}
		timeVal, err := varint.Read(br)
		if err != nil {
			return nil, err
		}
		samplesVal, err := varint.Read(br)
		if err != nil {
			return nil, err
		}
		var writesVal uint64
		if version >= 2 {
			writesVal, err = varint.Read(br)
			if err != nil {
				return nil, err
			}
		}
		presentVal, err := varint.Read(br)
		if err != nil {
			return nil, err
		}
		node := newNode(time.Unix(int64(timeVal), 0), int(depth), multiplier)
		if presentVal == 1 {
			node.present = true
		}
		node.samples = samplesVal
		node.writes = writesVal
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

func (s *Segment) Bytes() ([]byte, error) {
	b := bytes.Buffer{}
	if err := s.Serialize(&b); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func FromBytes(p []byte) (*Segment, error) {
	return Deserialize(bytes.NewReader(p))
}
