package v2

import (
	"bytes"
	"encoding/json"
	"io"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/util/varint"
)

func Marshal(s Segment) ([]byte, error) {
	var b bytes.Buffer
	w := varint.NewWriter()
	jb, err := json.Marshal(s.Meta)
	if err != nil {
		return nil, err
	}
	_, _ = w.Write(&b, uint64(s.CreatedAt.Unix()))
	_, _ = w.Write(&b, uint64(len(jb)))
	b.Write(jb)
	return b.Bytes(), nil
}

func Unmarshal(data []byte) (s Segment, err error) {
	b := bytes.NewReader(data)
	c, err := varint.Read(b)
	if err != nil {
		return s, err
	}
	l, err := varint.Read(b)
	if err != nil {
		return s, err
	}
	buf := make([]byte, l)
	if _, err = io.ReadAtLeast(b, buf, int(l)); err != nil {
		return s, err
	}
	if err = json.Unmarshal(buf, &s.Meta); err != nil {
		return s, err
	}
	s.CreatedAt = time.Unix(int64(c), 0)
	return s, nil
}
