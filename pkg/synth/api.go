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
	function(name string, nodes []*node) string
	// generates string representation of the program
	program(mainKey string, requires []string) string
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

// TODO: currently only supports ruby stacks
func splitIntoPathAndFunctionName(s string) (string, string) {
	arr := strings.Split(s, " - ")
	newName := simpleName(arr[len(arr)-1])
	if len(newName) == 0 {
		newName = "empty"
	}
	filePath := arr[0]
	if len(arr) == 1 || newName == "main" {
		filePath = "main.rb"
	}
	arr2 := strings.Split(filePath, ":")
	filePath = arr2[0]
	// newName += "_" + fmt.Sprintf("fn_%d", len(f.mapping))
	// f.mapping[name] = newName
	// f.mapping[name] = fmt.Sprintf("fn_%d_%s", len(f.mapping), newName)
	// logrus.Info(s, " ", filePath, " ", newName)
	return filePath, newName
}

func GenerateCode(t *tree.Tree, lang string) (string, error) {
	var g generator
	switch lang {
	case "ruby":
		g = newRubyGenerator()
	default:
		return "", fmt.Errorf("unsupported language: %s", lang)
	}

	fg := newFunctionNameSanitizer()
	pg := newPathSanitizer()
	kg := newKeyNamesSanitizer()

	// mapping from key to node
	allNodes := map[string]*node{}

	// mapping from function name to file name
	fileMapping := map[string][]string{
		"main.rb": {"main"},
	}

	// mapping from funcName to array of keys
	allFunctions := map[string][]string{}

	var mainKey string
	// j := 0

	t.IterateStacks(func(_ string, self uint64, stack []string) {
		newStack := []string{}
		for _, s := range stack {
			filePath, functionName := splitIntoPathAndFunctionName(s)
			filePath = pg.sanitize(filePath)
			functionName = fg.sanitize(functionName)
			newStack = append(newStack, functionName)
			exists := false
			for _, k := range fileMapping[filePath] {
				if k == functionName {
					exists = true
					break
				}
			}
			if !exists {
				fileMapping[filePath] = append(fileMapping[filePath], functionName)
			}
		}
		newStack = append(newStack, "main")
		stack = newStack

		childKey := ""
		for i, name := range stack {
			key := kg.sanitize(strings.Join(stack[i:], ";"))
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

	out := ""

	allFileNames := sort.StringSlice([]string{})
	for name := range fileMapping {
		allFileNames = append(allFileNames, name)
	}
	allFileNames.Sort()

	for filePath, functionNames := range fileMapping {
		fileContent := ""

		for _, name := range functionNames {
			nodeKeys := allFunctions[name]
			nodes := []*node{}
			for _, nodeKey := range nodeKeys {
				node := allNodes[nodeKey]
				nodes = append(nodes, node)
			}

			fileContent += g.function(name, nodes)
		}

		if filePath == "main.rb" {
			fileContent += g.program(mainKey, allFileNames)
		}

		out += newFile(filePath, fileContent)
	}

	return out, nil
}
