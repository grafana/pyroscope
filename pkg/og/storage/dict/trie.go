package dict

import (
	"bytes"
	"io"

	"github.com/grafana/pyroscope/pkg/og/util/varint"
)

// this implementation is a copy of another trie implementation in this repo
//   albeit slightly different
// TODO: maybe dedup them
type trieNode struct {
	label    []byte
	children []*trieNode
}

func newTrieNode(label []byte) *trieNode {
	return &trieNode{
		label:    label,
		children: make([]*trieNode, 0),
	}
}

func (tn *trieNode) insert(t2 *trieNode) {
	tn.children = append(tn.children, t2)
}

// TODO: too complicated, need to refactor / document this
func (tn *trieNode) findNodeAt(key []byte, vw varint.Writer, w io.Writer) {
	// log.Debug("findNodeAt")
	key2 := make([]byte, len(key))
	// TODO: remove
	copy(key2, key)
	key = key2

OuterLoop:
	for {
		// log.Debug("findNodeAt, key", string(key))

		if len(key) == 0 {
			// fn(tn)
			return
		}

		// 4 options:
		// trie:
		// foo -> bar
		// 1) no leads (baz)
		//    create a new child, call fn with it
		// 2) lead, key matches (foo)
		//    call fn with existing child
		// 3) lead, key matches, shorter (fo / fop)
		//    split existing child, set that as tn
		// 4) lead, key matches, longer (fooo)
		//    go to existing child, set that as tn

		leadIndex := -1
		for k, v := range tn.children {
			if v.label[0] == key[0] {
				leadIndex = k
			}
		}

		if leadIndex == -1 { // 1
			// log.Debug("case 1")
			newTn := newTrieNode(key)
			tn.insert(newTn)
			i := len(tn.children) - 1
			vw.Write(w, uint64(i))
			vw.Write(w, uint64(len(key)))
			// fn(newTn)
			return
		}

		leadKey := tn.children[leadIndex].label
		// log.Debug("lead key", string(leadKey))
		lk := len(key)
		llk := len(leadKey)
		for i := 0; i < lk; i++ {
			if i == llk { // 4 fooo / foo i = 3 llk = 3
				// log.Debug("case 4")
				tn = tn.children[leadIndex]
				key = key[llk:]
				vw.Write(w, uint64(leadIndex))
				vw.Write(w, uint64(llk))
				continue OuterLoop
			}
			if leadKey[i] != key[i] { // 3
				a := leadKey[:i] // ab
				b := leadKey[i:] // c
				newTn := newTrieNode(a)
				newTn.children = []*trieNode{tn.children[leadIndex]}
				tn.children[leadIndex].label = b
				tn.children[leadIndex] = newTn
				tn = newTn
				key = key[i:]

				vw.Write(w, uint64(leadIndex))
				vw.Write(w, uint64(i))
				continue OuterLoop
			}
		}
		// lk < llk
		if !bytes.Equal(key, leadKey) { // 3
			a := leadKey[:lk] // ab
			b := leadKey[lk:] // c
			newTn := newTrieNode(a)
			newTn.children = []*trieNode{tn.children[leadIndex]}
			tn.children[leadIndex].label = b
			tn.children[leadIndex] = newTn
			tn = newTn
			key = key[lk:]

			vw.Write(w, uint64(leadIndex))
			vw.Write(w, uint64(lk))
			continue OuterLoop
		}

		// 2
		vw.Write(w, uint64(leadIndex))
		vw.Write(w, uint64(len(leadKey)))
		return
	}
}
