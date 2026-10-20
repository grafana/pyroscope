package querybackend

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/pprof"
)

// Test_pprofAggregator_reappliesNodeBudget pins #5841 at the aggregation
// boundary: every partial report is individually within the node budget
// (the resolver applies it per partition), but their merge is not. The
// report built by the aggregator must re-apply the budget to the merged
// profile, the same way the tree path re-applies it after the merge.
func Test_pprofAggregator_reappliesNodeBudget(t *testing.T) {
	const (
		partitions   = 3
		perPartition = 10
		maxNodes     = int64(12)
	)
	// Compact profile builder: dense identifiers, one line per location.
	newPartial := func(p int) *profilev1.Profile {
		var (
			strs    = []string{""}
			strIdx  = map[string]int64{"": 0}
			fns     []*profilev1.Function
			locs    []*profilev1.Location
			samples []*profilev1.Sample
		)
		str := func(s string) int64 {
			if i, ok := strIdx[s]; ok {
				return i
			}
			i := int64(len(strs))
			strs = append(strs, s)
			strIdx[s] = i
			return i
		}
		fn := func(name string) *profilev1.Function {
			f := &profilev1.Function{
				Id:         uint64(len(fns) + 1),
				Name:       str(name),
				SystemName: str(name),
			}
			fns = append(fns, f)
			return f
		}
		loc := func(fs ...*profilev1.Function) *profilev1.Location {
			l := &profilev1.Location{
				Id:        uint64(len(locs) + 1),
				Address:   uint64(0x4000 + len(locs)),
				MappingId: 1,
			}
			for _, f := range fs {
				l.Line = append(l.Line, &profilev1.Line{FunctionId: f.Id})
			}
			locs = append(locs, l)
			return l
		}
		// Stacks are leaf-first, the pprof order.
		stack := func(value int64, fs ...*profilev1.Function) {
			s := &profilev1.Sample{Value: []int64{value}}
			for _, f := range fs {
				s.LocationId = append(s.LocationId, loc(f).Id)
			}
			samples = append(samples, s)
		}
		mid, root := fn("mid"), fn("root")
		for j := 0; j < perPartition; j++ {
			i := p*perPartition + j
			d := fn(fmt.Sprintf("d%d", i))
			c := fn(fmt.Sprintf("c%d", i))
			e := fn(fmt.Sprintf("e%d", i))
			a := fn(fmt.Sprintf("a%d", i))
			stack(int64(i+1), d, c, e, a, mid, root)
		}
		return &profilev1.Profile{
			SampleType:  []*profilev1.ValueType{{Type: str("samples"), Unit: str("count")}},
			PeriodType:  &profilev1.ValueType{Type: str("cpu"), Unit: str("nanoseconds")},
			Sample:      samples,
			Location:    locs,
			Function:    fns,
			Mapping:     []*profilev1.Mapping{{Id: 1, Filename: str("app.bin"), HasFunctions: true}},
			StringTable: strs,
		}
	}

	a := &pprofAggregator{}
	for p := 0; p < partitions; p++ {
		partial := newPartial(p)
		require.NoError(t, a.aggregate(&queryv1.Report{
			Pprof: &queryv1.PprofReport{
				Query: &queryv1.PprofQuery{MaxNodes: maxNodes},
				Pprof: pprof.MustMarshal(partial, true),
			},
		}))
	}

	report := a.build()
	require.NotNil(t, report.Pprof)
	var got profilev1.Profile
	require.NoError(t, pprof.Unmarshal(report.Pprof.Pprof, &got))

	// No value is lost by the merge or the enforcement.
	var sum int64
	for _, s := range got.Sample {
		sum += s.Value[0]
	}
	assert.Equal(t, int64(465), sum)
	// The budget is re-applied: 30 branches with 4 frames each plus the
	// trunk merge to 122 functions; the built report keeps 42.
	assert.Len(t, got.Function, 42)
	// The synthetic frame is present and carries the cut values.
	var stub int64
	for _, s := range got.Sample {
		for _, id := range s.LocationId {
			if int(id) > 0 && int(id) <= len(got.Location) {
				l := got.Location[id-1]
				if len(l.Line) == 1 {
					f := got.Function[l.Line[0].FunctionId-1]
					if f.Name < int64(len(got.StringTable)) && got.StringTable[f.Name] == "other" {
						stub += s.Value[0]
						break
					}
				}
			}
		}
	}
	assert.Equal(t, int64(378), stub)
}
