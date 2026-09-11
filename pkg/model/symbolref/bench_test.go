package symbolref_test

// Benchmarks below cover Table's intern/merge throughput and
// ResultBuilder's marshal-time compaction. Rebuild's own memory/throughput
// benchmark, and a wire-size-vs-literal-encoding benchmark, are deferred
// until Rebuild is implemented.

import (
	"fmt"
	"testing"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/model/symbolref"
)

// benchNames is the fixed set of resolved names every benchmark variant
// interns alongside its unresolved addresses, approximating a realistic
// resolved/unresolved mix.
var benchNames = []string{
	"main", "runtime.gopark", "runtime.mallocgc", "net/http.(*conn).serve", "runtime.selectgo",
}

// buildBenchTree interns benchNames plus numAddresses addresses (spread
// across numBuildIDs build IDs, starting at addrBase) into table, and
// inserts one two-level stack per address (rooted at a name, cycling
// through benchNames) so every interned ref is referenced by the tree.
func buildBenchTree(table *symbolref.Table, numAddresses, numBuildIDs int, addrBase uint64) *model.LocationRefNameTree {
	nameRefs := make([]model.LocationRefName, len(benchNames))
	for i, n := range benchNames {
		nameRefs[i] = table.InternName(n)
	}
	tree := new(model.LocationRefNameTree)
	for a := range numAddresses {
		buildID := fmt.Sprintf("build-%d", a%numBuildIDs)
		ref := table.InternUnresolved(buildID, "binary-"+buildID, addrBase+uint64(a))
		tree.InsertStack(1, nameRefs[a%len(nameRefs)], ref)
	}
	return tree
}

// benchmarkInternAndAdd measures combined InternName+InternUnresolved+Add
// throughput at a given cardinality.
func benchmarkInternAndAdd(b *testing.B, numAddresses, numBuildIDs int) {
	// Pre-build the "second table"'s wire form once: Add's cost scales with
	// its size, not its content, so it need not vary per iteration. Its
	// addresses are offset past dst's own range so each iteration's Add
	// call grows dst by numAddresses new entries (a realistic mixed-dataset
	// merge) rather than only exercising the dedup path.
	source := buildPartial(func(table *symbolref.Table) *model.LocationRefNameTree {
		return buildBenchTree(table, numAddresses, numBuildIDs, uint64(numAddresses))
	})

	inputSize := numAddresses
	b.ReportAllocs()
	for b.Loop() {
		dst := symbolref.NewTable()
		for _, n := range benchNames {
			dst.InternName(n)
		}
		for a := range numAddresses {
			buildID := fmt.Sprintf("build-%d", a%numBuildIDs)
			dst.InternUnresolved(buildID, "binary-"+buildID, uint64(a))
		}
		if _, err := dst.Add(source.pb); err != nil {
			b.Fatal(err)
		}
	}

	opsPerSec := float64(b.N*inputSize) / b.Elapsed().Seconds()
	b.ReportMetric(opsPerSec, "ops/sec")
	b.ReportMetric(float64(inputSize), "input_size")
}

// BenchmarkInternAndAdd_small: single-dataset, lightly unsymbolized.
func BenchmarkInternAndAdd_small(b *testing.B) { benchmarkInternAndAdd(b, 100, 1) }

// BenchmarkInternAndAdd_medium: typical mixed-language service.
func BenchmarkInternAndAdd_medium(b *testing.B) { benchmarkInternAndAdd(b, 10_000, 10) }

// BenchmarkInternAndAdd_large: eBPF whole-host profile, worst case for
// deferred truncation.
func BenchmarkInternAndAdd_large(b *testing.B) { benchmarkInternAndAdd(b, 1_000_000, 100) }

// BenchmarkResultBuilderBuild builds a 100,000-node LocationRefNameTree with
// a realistic resolved/unresolved mix, and measures
// tree.Bytes(0, rb.KeepRef) + rb.Build(pb) wall time and allocations.
func BenchmarkResultBuilderBuild(b *testing.B) {
	const (
		numNodes    = 100_000
		numBuildIDs = 10
	)
	table := symbolref.NewTable()
	tree := buildBenchTree(table, numNodes, numBuildIDs, 0)

	b.ReportAllocs()
	for b.Loop() {
		rb := table.ResultBuilder()
		tree.Bytes(0, rb.KeepRef)
		pb := new(queryv1.SymbolRefTable)
		rb.Build(pb)
	}

	b.ReportMetric(float64(numNodes), "input_size")
}
