package serialization

import (
	"bufio"
	"encoding/json"
	"io"

	"github.com/grafana/pyroscope/pkg/og/util/varint"
)

func WriteMetadata(w io.Writer, metadata map[string]interface{}) error {
	b, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	varint.Write(w, uint64(len(b)))
	w.Write(b)
	return nil
}

func ReadMetadata(br *bufio.Reader) (map[string]interface{}, error) {
	l, err := varint.Read(br)
	if err != nil {
		return nil, err
	}
	jsonBuf := make([]byte, l)
	_, err = io.ReadAtLeast(br, jsonBuf, int(l))
	if err != nil {
		return nil, err
	}
	var metadata map[string]interface{}
	err = json.Unmarshal(jsonBuf, &metadata)
	if err != nil {
		return nil, err
	}
	return metadata, nil
}
