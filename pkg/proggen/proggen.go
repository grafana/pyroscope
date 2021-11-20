package proggen

import (
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"errors"
	"log"
	mapset "github.com/deckarep/golang-set"
)

type Function struct {
	name string
	workAmount uint64
	calls []string
	cpuUtilizationTarget float64
	loopCalls bool
}

type Program struct {
	functions []Function
	entryFunctionName string
}

func hash(key []byte) string {
	checksum := sha256.Sum224(key)
	hexString := hex.EncodeToString(checksum[:])
	return hexString
}

func generateFunctionName(key []byte) string {
	log.Printf("string(key) %s", string(key))
	hashString := hash(key)
	prefix := "f"
	functionName := prefix + hashString
	log.Printf("functionName %s", functionName)
	return functionName
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
	name := generateFunctionName(copiedByteSlice(key))
	calls := make([]string, 0)
	for _, callKey := range childKeys {
		callName := generateFunctionName(copiedByteSlice(callKey))
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

func copiedByteSlice(oldSlice []byte) []byte {
	newSlice := make([]byte, len(oldSlice))
	copy(newSlice, oldSlice)
	return newSlice
}

func treeToFunctions(tree *tree.Tree, cpuUtilizationTarget float64) []Function {
	functions := make([]Function, 0)
	i := 0
	k := 0
	seenChildKeys := mapset.NewSet()
	seenNodeKeys := mapset.NewSet()
	tree.IterateWithChildKeys(func(key []byte, workAmount uint64, childKeys [][]byte) {
		i = i + 1
		k = k + len(childKeys)
		for _, childKey := range childKeys {
			seenChildKeys.Add(string(copiedByteSlice(childKey)))
		}
		seenNodeKeys.Add(string(copiedByteSlice(key)))
		function := nodeWithChildKeysToFunction(copiedByteSlice(key), workAmount, childKeys, cpuUtilizationTarget)
		functions = append(functions, function)
	})
	log.Printf("parents visited %v", i)
	log.Printf("sum of len(childKeys) %v", k)
	log.Printf("seenChildKeys.Cardinality() %v", seenChildKeys.Cardinality())
	log.Printf("seenChildKeys.Difference(seenNodeKeys).Cardinality() %v", seenChildKeys.Difference(seenNodeKeys).Cardinality())
	log.Printf("seenChildKeys.Difference(seenNodeKeys) %v", seenChildKeys.Difference(seenNodeKeys))
	log.Printf("seenNodeKeys.Difference(seenChildKeys).Cardinality() %v", seenNodeKeys.Difference(seenChildKeys).Cardinality())
	log.Printf("seenNodeKeys.Difference(seenChildKeys) %v", seenNodeKeys.Difference(seenChildKeys))
	return functions
}

func TreeToProgram(tree *tree.Tree, cpuUtilizationTarget float64) Program {
	entryFunctionKey := tree.RootKey()
	entryFunctionName := generateFunctionName(entryFunctionKey)
	return Program {
		functions: treeToFunctions(tree, cpuUtilizationTarget),
		entryFunctionName: entryFunctionName,
	}
}

func indentation() string {
	return "\t"
}

// TODO Accomodate Function.cpuUtilizationTarget
func (f *Function) toBlock() string {
	callBlock := ""
	if len(f.calls) != 0 {
		callBlock = strings.Join(f.calls, "()\n" + indentation()) + "()"
		if f.loopCalls {
			callBlock = "for {" + "\n" + indentation() + callBlock + "\n" + indentation() + "}"
		}
	}
	workBlock := ""
	if f.workAmount != 0 {
		workBlock = fmt.Sprintf(`
	for i := 0; i < %d; i++ {
		select {
			case durationToSleep, shouldSleep := <-durationToSleepChannel:
				if shouldSleep {
					time.Sleep(durationToSleep)
				}
			default:
		}
	}
			`, f.workAmount)
		workBlock = strings.TrimSpace(workBlock)
	}
	nonEmptySubBlocks := make([]string, 0)
	for _, subBlock := range []string{workBlock, callBlock} {
		if subBlock != "" {
			nonEmptySubBlocks = append(nonEmptySubBlocks, subBlock)
		}
	}
	subBlocks :=
		"\n" + indentation() +
		strings.Join(nonEmptySubBlocks, "\n" + indentation()) +
		"\n"
	return fmt.Sprintf(`func %s() {%s}`, f.name, subBlocks)
}

func generateGoCode(program *Program) string {
	blocks := make([]string, 0)

	packageBlock := "package main"
	blocks = append(blocks, packageBlock)

	importBlock := "import \"time\""
	//importBlock = importBlock + "\n" + "import \"fmt\""
	blocks = append(blocks, importBlock)


	fullCycleInMillis := 100
	// TODO currently cpuUtilizationTarget is a property of Function; refactor to make it Program's property
	cpuUtilizationTarget := 0.80
	setupFunctionName := "setupFunction"
	timerInitBlock := fmt.Sprintf(`
var fullCycleInMillis = %d
var cpuUtilizationTarget = %f
var durationToWork = time.Duration(int(cpuUtilizationTarget * float64(fullCycleInMillis))) * time.Millisecond
var durationToSleep = time.Duration(int((1 - cpuUtilizationTarget) * float64(fullCycleInMillis))) * time.Millisecond
var durationToSleepChannel = make(chan time.Duration, 1)
func switchToSleep() {
	durationToSleepChannel <- durationToSleep
	time.AfterFunc(durationToWork, switchToSleep)
}
var shouldDoSetup = true
func %s() {
	if shouldDoSetup {
		time.AfterFunc(durationToWork, switchToSleep)
		shouldDoSetup = false
	}
}
		`, fullCycleInMillis, cpuUtilizationTarget, setupFunctionName)

	blocks = append(blocks, timerInitBlock)

	functionBlocks := make([]string, 0)

	for _, function := range program.functions {
		functionBlock := function.toBlock()
		functionBlocks = append(functionBlocks, functionBlock)
	}

	blocks = append(blocks, functionBlocks...)

	mainFunction := Function {
		name: "main",
		calls: []string{program.entryFunctionName, setupFunctionName},
		workAmount: 0,
		cpuUtilizationTarget: 0.75,
		loopCalls: true,
	}

	mainBlock := mainFunction.toBlock()

	blocks = append(blocks, mainBlock)

	code := strings.Join(blocks, "\n")

	return code
}

type Language string

const (
	InvalidTarget Language = ""
	Go = "go"
	Ruby = "ruby"
)

func knownLanguages() []Language {
	return []Language {Go, Ruby}
}

func (lang Language) String() string {
	return string(lang)
}

func LookupLanguage(s string) (Language, error) {
	s = strings.TrimSpace(s)
	if (s == "") {
		return InvalidTarget, errors.New("language unspecified")
	}
	s = strings.ToLower(s)
	matches := func(lang Language, s string) bool {
		return lang.String() == s
	}
	for _, lang := range knownLanguages() {
		if matches(lang, s) {
			return lang, nil
		}
	}
	return InvalidTarget, errors.New("language unknown")
}

func (program *Program) ToCode(lang Language) (string, error) {
	switch lang {
	case Go:
		return generateGoCode(program), nil
	default:
		return "", errors.New("invalid language passed")
	}
}
