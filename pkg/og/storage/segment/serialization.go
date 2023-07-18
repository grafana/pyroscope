package segment

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"time"

	"github.com/grafana/pyroscope/pkg/og/storage/metadata"
	"github.com/grafana/pyroscope/pkg/og/util/serialization"
	"github.com/grafana/pyroscope/pkg/og/util/varint"
)

// serialization format version. it's not very useful right now, but it will be in the future
const currentVersion = 3

func (s *Segment) populateFromMetadata(mdata map[string]interface{}) {
	if v, ok := mdata["sampleRate"]; ok {
		s.sampleRate = uint32(v.(float64))
	}
	if v, ok := mdata["spyName"]; ok {
		s.spyName = v.(string)
	}
	if v, ok := mdata["units"]; ok {
		s.units = metadata.Units(v.(string))
	}
	if v, ok := mdata["aggregationType"]; ok {
		s.aggregationType = metadata.AggregationType(v.(string))
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
	if _, err := vw.Write(w, currentVersion); err != nil {
		return err
	}
	if err := serialization.WriteMetadata(w, s.generateMetadata()); err != nil {
		return err
	}

	if s.root == nil {
		return nil
	}

	s.serialize(w, vw, s.root)

	return s.watermarks.serialize(w)
}

func (s *Segment) serialize(w io.Writer, vw varint.Writer, n *streeNode) {
	vw.Write(w, uint64(n.depth))
	vw.Write(w, uint64(n.time.Unix()))
	vw.Write(w, n.samples)
	vw.Write(w, n.writes)
	p := uint64(0)
	if n.present {
		p = 1
	}
	vw.Write(w, p)

	// depth
	// time
	// keyInChunks
	// children
	l := 0
	for _, v := range n.children {
		if v != nil {
			l++
		}
	}

	vw.Write(w, uint64(l))
	for _, v := range n.children {
		if v != nil {
			s.serialize(w, vw, v)
		}
	}
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

	mdata, err := serialization.ReadMetadata(br)
	if err != nil {
		return nil, err
	}
	s.populateFromMetadata(mdata)

	// In some cases, there can be no nodes.
	if br.Buffered() == 0 {
		return s, nil
	}

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

	if version >= 3 {
		if err = deserializeWatermarks(br, &s.watermarks); err != nil {
			return nil, err
		}
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

func (w watermarks) serialize(dst io.Writer) error {
	vw := varint.NewWriter()
	if _, err := vw.Write(dst, uint64(w.absoluteTime.UTC().Unix())); err != nil {
		return err
	}
	if _, err := vw.Write(dst, uint64(len(w.levels))); err != nil {
		return err
	}
	for k, v := range w.levels {
		if _, err := vw.Write(dst, uint64(k)); err != nil {
			return err
		}
		if _, err := vw.Write(dst, uint64(v.UTC().Unix())); err != nil {
			return err
		}
	}
	return nil
}

func deserializeWatermarks(r io.ByteReader, w *watermarks) error {
	a, err := varint.Read(r)
	if err != nil {
		return err
	}
	w.absoluteTime = time.Unix(int64(a), 0).UTC()
	l, err := varint.Read(r)
	if err != nil {
		return err
	}
	levels := int(l)
	for i := 0; i < levels; i++ {
		k, err := varint.Read(r)
		if err != nil {
			return err
		}
		v, err := varint.Read(r)
		if err != nil {
			return err
		}
		w.levels[int(k)] = time.Unix(int64(v), 0).UTC()
	}
	return nil
}
