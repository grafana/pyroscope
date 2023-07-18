package transporttrie

import "fmt"

func (t *Trie) String() string {
	str := "trie:\n"
	nodes := []*trieNode{t.root}
	levels := []int{0}
	parentNames := make([][]byte, 1)
	parentNames[0] = make([]byte, 0)

	for len(nodes) > 0 {
		node := nodes[0]
		level := levels[0]
		parentName := parentNames[0]

		nodes = nodes[1:]
		levels = levels[1:]
		parentNames = parentNames[1:]

		prefix := "-"
		for i := 0; i < level; i++ {
			prefix += "-"
		}
		str += fmt.Sprintf("%s %q %q %q\n", prefix, parentName, node.name, node.value)
		names := [][]byte{}
		for _, v := range node.children {
			names = append(names, v.name)
		}
		parentNames = append(names, parentNames...)
		nodes = append(node.children, nodes...)
		for i := 0; i < len(node.children); i++ {
			levels = append([]int{level + 1}, levels...)
		}
	}
	return str
}
