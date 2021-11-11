package proggen

import (
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type Function struct {
	name string
	workAmount uint64
	calls []string
}

type Program struct {
	functions []Function
	entryFunctionName string
}

func hash(key []byte) string {
	checksum := sha256.Sum224(key)
	return hex.EncodeToString(checksum[:])
}

func nodeWithChildKeysToFunction(
	key []byte,
	workAmount uint64,
	childKeys [][]byte,
) Function {
	name := hash(key)
	calls := make([]string, 0)
	for _, callKey := range childKeys {
		callName := hash(callKey)
		calls = append(calls, callName)
	}
	function := Function{ name: name, workAmount: workAmount, calls: calls }
	return function
}

func treeToFunctions(tree *tree.Tree) []Function {
	functions := make([]Function, 0)
	tree.IterateWithChildKeys(func(key []byte, workAmount uint64, childKeys [][]byte) {
		function := nodeWithChildKeysToFunction(key, workAmount, childKeys)
		functions = append(functions, function)
	})
	return functions
}

func treeToProgram(tree *tree.Tree) Program {
	entryFunctionKey := tree.RootKey()
	entryFunctionName := hash(entryFunctionKey)
	return Program { functions: treeToFunctions(tree), entryFunctionName: entryFunctionName }
}

func (f *Function) toBlock() string {
	callBlock := strings.Join(f.calls, "()\n") + "()"
	return fmt.Sprintf(`
func %s() {
	for i := 0; i < %d; i++ {
		// noop
	}
	%s
}
		`,
		f.name,
		f.workAmount,
		callBlock,
	)
}

func generateGoCode(program Program) string {
	blocks := make([]string, 0)

	for _, function := range program.functions {
		blocks = append(blocks, function.toBlock())
	}

	entryBlock := program.entryFunctionName + "()"

	blocks = append(blocks, entryBlock)

	code := strings.Join(blocks, "\n")

	return code
}
