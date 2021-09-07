package dict

import (
	"bytes"
	"io"
	"sync"

	"github.com/pyroscope-io/pyroscope/pkg/util/varint"
)

type (
	Key   []byte
	Value []byte
)

func New() *Dict {
	return &Dict{
		root: newTrieNode([]byte{}),
	}
}

type Dict struct {
	m    sync.RWMutex
	root *trieNode
}

func (td *Dict) PutValue(val Value, key io.Writer) {
	td.m.Lock()
	defer td.m.Unlock()
	td.root.findNodeAt(val, key)
}

func (td *Dict) GetValue(key Key, value io.Writer) bool {
	td.m.RLock()
	defer td.m.RUnlock()
	return td.writeValue(key, value)
}

func (td *Dict) Put(val Value) Key {
	td.m.Lock()
	defer td.m.Unlock()
	var buf bytes.Buffer
	td.root.findNodeAt(val, &buf)
	return buf.Bytes()
}

func (td *Dict) Get(key Key) (Value, bool) {
	td.m.RLock()
	defer td.m.RUnlock()
	var labelBuf bytes.Buffer
	if td.writeValue(key, &labelBuf) {
		return labelBuf.Bytes(), true
	}
	return nil, false
}

func (td *Dict) writeValue(key Key, w io.Writer) bool {
	r := bytes.NewReader(key)
	tn := td.root
	for {
		v, err := varint.Read(r)
		if err != nil {
			return true
		}
		if int(v) >= len(tn.children) {
			return false
		}

		label := tn.children[v].label
		_, _ = w.Write(label)
		tn = tn.children[v]

		expectedLen, _ := varint.Read(r)
		for len(label) < int(expectedLen) {
			if len(tn.children) == 0 {
				return false
			}
			label2 := tn.children[0].label
			_, _ = w.Write(label2)
			expectedLen -= uint64(len(label2))
			tn = tn.children[0]
		}
	}
}
