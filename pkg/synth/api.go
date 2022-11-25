package synth

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type call struct {
	name      string
	parameter string
}

type node struct {
	funcName string
	key      string
	self     uint64
	calls    []*call
}

type generator interface {
	// adds a function definition that performs some work and calls other functions
	function(name string, nodes []*node)
	// generates string representation of the program
	program(mainKey string) string
}

// type invocation struct {
// 	name  string
// 	param int
// }

// type function struct {
// 	name        string
// 	invocations []invocation
// 	count       int
// }

func GenerateCode(t *tree.Tree, lang string) (string, error) {
	var g generator
	switch lang {
	case "ruby":
		g = newRubyGenerator()
	default:
		return "", fmt.Errorf("unsupported language: %s", lang)
	}

	fg := newFunctionNameGenerator()
	kg := newKeyNamesGenerator()

	// mapping from key to node
	allNodes := map[string]*node{}

	// mapping from funcName to array of keys
	allFunctions := map[string][]string{}

	var mainKey string
	j := 0

	t.IterateStacks(func(_ string, self uint64, stack []string) {
		newStack := []string{}
		for _, s := range stack {
			if s == "other" {
				s = fmt.Sprintf("%s_%d", s, j)
				j++
			}
			newStack = append(newStack, fg.functionName(s))
		}
		newStack = append(newStack, "main")
		stack = newStack

		childKey := ""
		for i, name := range stack {
			key := kg.key(strings.Join(stack[i:], ";"))
			if name == "main" {
				mainKey = key
			}
			if _, ok := allNodes[key]; !ok {
				allNodes[key] = &node{
					key:      key,
					funcName: name,
				}
			}
			if _, ok := allFunctions[name]; !ok {
				allFunctions[name] = []string{}
			}
			exists := false
			for _, k := range allFunctions[name] {
				if k == key {
					exists = true
					break
				}
			}
			if !exists {
				allFunctions[name] = append(allFunctions[name], key)
			}
			parentNode := allNodes[key]
			if i == 0 && self > 0 {
				allNodes[key].self = self
			}
			if childKey != "" {
				childNode := allNodes[childKey]
				exists := false
				for _, c := range parentNode.calls {
					if c.name == childNode.funcName {
						exists = true
						break
					}
				}
				if !exists {
					parentNode.calls = append(parentNode.calls, &call{
						name:      childNode.funcName,
						parameter: childKey,
					})
				}
			}
			childKey = key
		}
	})

	allFunctionNames := sort.StringSlice([]string{})
	for name, _ := range allFunctions {
		allFunctionNames = append(allFunctionNames, name)
	}
	allFunctionNames.Sort()

	for _, name := range allFunctionNames {
		nodeKeys := allFunctions[name]
		nodes := []*node{}
		for _, nodeKey := range nodeKeys {
			node := allNodes[nodeKey]
			nodes = append(nodes, node)
		}

		g.function(name, nodes)
	}

	return g.program(mainKey), nil
}
