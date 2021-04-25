package transporttrie

func (originalTrie *Trie) Diff(srcTrie *Trie) *Trie {
	dstTrie := originalTrie.Clone(1, 1)
	dstTrie.root = originalTrie.root.clone()

	srcNodes := []*trieNode{srcTrie.root}
	dstNodes := []*trieNode{dstTrie.root}

	for len(srcNodes) > 0 {
		st := srcNodes[0]
		srcNodes = srcNodes[1:]

		dt := dstNodes[0]
		dstNodes = dstNodes[1:]

		for _, srcChildNode := range st.children {
			dt.findNodeAt(srcChildNode.name, func(dstChildNode *trieNode) {
				if srcChildNode.value > dstChildNode.value {
					dstChildNode.value = 0
				} else {
					dstChildNode.value -= srcChildNode.value
				}
				srcNodes = append([]*trieNode{srcChildNode}, srcNodes...)
				dstNodes = append([]*trieNode{dstChildNode}, dstNodes...)
			})
		}
	}
	return dstTrie
}
