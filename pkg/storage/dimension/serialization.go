package dimension

import (
	"bufio"
	"bytes"
	"io"

	"github.com/pyroscope-io/pyroscope/pkg/util/varint"
)

// serialization format version. it's not very useful right now, but it will be in the future
const currentVersion = 1

func (s *Dimension) Serialize(w io.Writer) error {
	varint.Write(w, currentVersion)

	for _, k := range s.keys {
		varint.Write(w, uint64(len(k)))
		if err != nil {
			return nil
		} else {
			w.Write([]byte(k))
		}
	}
	return nil
}

func Deserialize(r io.Reader) (*Dimension, error) {
	s := New()

	br := bufio.NewReader(r) // TODO if it's already a bytereader skip

	// reads serialization format version, see comment at the top
	_, err := varint.Read(br)
	if err != nil {
		return nil, err
	}

	for {
		keyLen, err := varint.Read(br)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		keyBuf := make([]byte, keyLen) // TODO: there are better ways to do this?
		_, err = io.ReadAtLeast(br, keyBuf, int(keyLen))
		if err != nil {
			return nil, err
		}

		s.keys = append(s.keys, key(keyBuf))
	}

	return s, nil
}

func (t *Dimension) Bytes() []byte {
	b := bytes.Buffer{}
	t.Serialize(&b)
	return b.Bytes()
}

func FromBytes(p []byte) *Dimension {
	t, _ := Deserialize(bytes.NewReader(p))
	return t
}
