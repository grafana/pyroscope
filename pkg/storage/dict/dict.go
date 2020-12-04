package dict

import (
	"bytes"

	"github.com/petethepig/pyroscope/pkg/util/varint"
)

type Key []byte
type Value []byte

func New() *Dict {
	return &Dict{
		root: newTrieNode([]byte{}),
	}
}

type Dict struct {
	root *trieNode
}

func (td *Dict) Get(key Key) (Value, bool) {
	r := bytes.NewReader(key)
	tn := td.root
	labelBuf := []byte{}
	for {
		v, err := varint.Read(r)
		if err != nil {
			return Value(labelBuf), true
		}

		if int(v) >= len(tn.children) {
			return nil, false
		}
		label := tn.children[v].label
		labelBuf = append(labelBuf, label...)
		tn = tn.children[v]

		expectedLen, err := varint.Read(r)
		for len(label) < int(expectedLen) {
			label2 := tn.children[0].label
			labelBuf = append(labelBuf, label2...)
			expectedLen -= uint64(len(label2))
			tn = tn.children[0]
		}
	}
}

func (td *Dict) Put(val Value) Key {
	buf := &bytes.Buffer{}
	td.root.findNodeAt(val, buf)
	return Key(buf.Bytes())
}
