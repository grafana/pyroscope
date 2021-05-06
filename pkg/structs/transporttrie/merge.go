package transporttrie

import "github.com/pyroscope-io/pyroscope/pkg/structs/merge"

func (dstTrie *Trie) Merge(srcTrieI merge.Merger) {
	srcTrie := srcTrieI.(*Trie)
	srcNodes := []*trieNode{srcTrie.root}
	dstNodes := []*trieNode{dstTrie.root}

	for len(srcNodes) > 0 {
		st := srcNodes[0]
		srcNodes = srcNodes[1:]

		dt := dstNodes[0]
		dstNodes = dstNodes[1:]

		for _, srcChildNode := range st.children {
			dt.findNodeAt(srcChildNode.name, func(dstChildNode *trieNode) {
				if srcChildNode.value > 0 {
					dstChildNode.value = mergeFunc(dstChildNode.value, srcChildNode.value, dstTrie, srcTrie)
				}
				srcNodes = append([]*trieNode{srcChildNode}, srcNodes...)
				dstNodes = append([]*trieNode{dstChildNode}, dstNodes...)
			})
		}
	}
}
