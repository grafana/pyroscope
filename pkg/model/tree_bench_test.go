package model

import (
	"fmt"
	"testing"
	"unique"
)

// Generate test dataset
func generateDeepWideDataset(numStacks, depth int) ([]int64, [][]FunctionName, [][]unique.Handle[string]) {
	values := make([]int64, numStacks)
	fnStacks := make([][]FunctionName, numStacks)
	handleStacks := make([][]unique.Handle[string], numStacks)

	numUniqueFrames := 10000
	frameNames := make([]string, numUniqueFrames)
	frameFNs := make([]FunctionName, numUniqueFrames)
	frameHandles := make([]unique.Handle[string], numUniqueFrames)

	for i := range numUniqueFrames {
		name := fmt.Sprintf("github.com/grafana/pyroscope/v2/pkg/long_package_name_component_%d.(*VeryLongStructNameForProfilingPerformanceBenchmark_%d).DeepStackMethodCallImplementationDetail_%d_ExtraSuffix_ToMakeNameExtremelyLongAndRealistic_ForProductionProfilingData", i%100, i%200, i)
		frameNames[i] = name
		frameFNs[i] = FunctionName(name)
		frameHandles[i] = unique.Make(name)
	}

	for s := range numStacks {
		values[s] = int64((s % 100) + 1)
		fnStack := make([]FunctionName, depth)
		handleStack := make([]unique.Handle[string], depth)
		for d := 0; d < depth; d++ {
			var idx int
			if d < 10 {
				idx = d // common root prefix frames
			} else {
				idx = (s*31 + d*17) % numUniqueFrames
			}
			fnStack[d] = frameFNs[idx]
			handleStack[d] = frameHandles[idx]
		}
		fnStacks[s] = fnStack
		handleStacks[s] = handleStack
	}

	return values, fnStacks, handleStacks
}

// Benchmark tree insertion using standard FunctionName stack slices
func BenchmarkTreeInsert_FunctionName(b *testing.B) {
	values, fnStacks, _ := generateDeepWideDataset(50000, 150)

	b.ReportAllocs()

	for b.Loop() {
		t := new(FunctionNameTree)
		for s := 0; s < len(fnStacks); s++ {
			t.InsertStack(values[s], fnStacks[s]...)
		}
	}
}

// Benchmark tree insertion using nodeBuffer with FunctionName stack slices
func BenchmarkTreeInsert_FunctionNameBuffered(b *testing.B) {
	values, fnStacks, _ := generateDeepWideDataset(10000, 150)

	b.ReportAllocs()

	for b.Loop() {
		buf := newNodeBuffer[FunctionName](50000)
		t := new(FunctionNameTree)
		for s := 0; s < len(fnStacks); s++ {
			t.InsertStackBuf(buf, values[s], fnStacks[s]...)
		}
	}
}

// Benchmark tree insertion using pre-interned unique.Handle[string] stacks
func BenchmarkTreeInsert_Handles(b *testing.B) {
	values, _, handleStacks := generateDeepWideDataset(50000, 50)

	b.ReportAllocs()

	for b.Loop() {
		t := new(FunctionNameTree)
		for s := 0; s < len(handleStacks); s++ {
			t.InsertStackHandles(values[s], handleStacks[s]...)
		}
	}
}

// Benchmark tree insertion using nodeBuffer with pre-interned unique.Handle[string] stacks
func BenchmarkTreeInsert_HandlesBuffered(b *testing.B) {
	values, _, handleStacks := generateDeepWideDataset(50000, 150)

	b.ReportAllocs()

	for b.Loop() {
		buf := newNodeBuffer[FunctionName](50000)
		t := new(FunctionNameTree)
		for s := 0; s < len(handleStacks); s++ {
			t.InsertStackHandlesBuf(buf, values[s], handleStacks[s]...)
		}
	}
}
