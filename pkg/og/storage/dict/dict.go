package dict

import (
	"bytes"
	"io"
	"sync"

	"github.com/grafana/pyroscope/pkg/og/util/varint"
	"github.com/valyala/bytebufferpool"
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

func (t *Dict) GetValue(key Key, value io.Writer) bool {
	t.m.RLock()
	defer t.m.RUnlock()
	return t.readValue(key, value)
}

func (t *Dict) Get(key Key) (Value, bool) {
	t.m.RLock()
	defer t.m.RUnlock()
	var labelBuf bytes.Buffer
	if t.readValue(key, &labelBuf) {
		return labelBuf.Bytes(), true
	}
	return nil, false
}

func (t *Dict) readValue(key Key, w io.Writer) bool {
	r := bytes.NewReader(key)
	tn := t.root
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

var writerPool = sync.Pool{New: func() any { return varint.NewWriter() }}

func (t *Dict) PutValue(val Value, dst io.Writer) {
	t.m.Lock()
	defer t.m.Unlock()
	vw := writerPool.Get().(varint.Writer)
	defer writerPool.Put(vw)
	t.root.findNodeAt(val, vw, dst)
}

var bufferPool bytebufferpool.Pool

func (t *Dict) Put(val Value) Key {
	b := bufferPool.Get()
	defer bufferPool.Put(b)
	t.PutValue(val, b)
	k := make([]byte, b.Len())
	copy(k, b.B)
	return k
}
