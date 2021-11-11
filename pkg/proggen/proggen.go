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
	cpuUtilizationTarget float64
}

type Program struct {
	functions []Function
	entryFunctionName string
}

func hash(key []byte) string {
	checksum := sha256.Sum224(key)
	return hex.EncodeToString(checksum[:])
}

// To be used with Tree.IterateWithChildKeys.
// Essentially converts a treeNode into a Function.
// Notice the hashing of treeNode keys. The treeNode key is considered a
// universal identifier of every callable in the program to be generated.
// Keys are hashed into alphanumeric strings to be used as function names.
func nodeWithChildKeysToFunction(
	key []byte,
	workAmount uint64,
	childKeys [][]byte,
	cpuUtilizationTarget float64,
) Function {
	name := hash(key)
	calls := make([]string, 0)
	for _, callKey := range childKeys {
		callName := hash(callKey)
		calls = append(calls, callName)
	}
	function := Function{
			name: name,
			workAmount: workAmount,
			calls: calls,
			cpuUtilizationTarget: cpuUtilizationTarget,
		}
	return function
}

func treeToFunctions(tree *tree.Tree, cpuUtilizationTarget float64) []Function {
	functions := make([]Function, 0)
	tree.IterateWithChildKeys(func(key []byte, workAmount uint64, childKeys [][]byte) {
		function := nodeWithChildKeysToFunction(key, workAmount, childKeys, cpuUtilizationTarget)
		functions = append(functions, function)
	})
	return functions
}

func treeToProgram(tree *tree.Tree, cpuUtilizationTarget float64) Program {
	entryFunctionKey := tree.RootKey()
	entryFunctionName := hash(entryFunctionKey)
	return Program {
		functions: treeToFunctions(tree, cpuUtilizationTarget),
		entryFunctionName: entryFunctionName,
	}
}

// TODO Accomodate Function.cpuUtilizationTarget
func (f *Function) toBlock() string {
	callBlock := strings.Join(f.calls, "()\n") + "()"
	untrimmed := fmt.Sprintf(`
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
	return strings.TrimSpace(untrimmed)
}

func generateGoCode(program Program) string {
	blocks := make([]string, 0)

	for _, function := range program.functions {
		block := function.toBlock()
		blocks = append(blocks, block)
	}

	entryBlock := program.entryFunctionName + "()"

	blocks = append(blocks, entryBlock)

	code := strings.Join(blocks, "\n")

	return code
}
