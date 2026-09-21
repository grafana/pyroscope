package pprof

import (
	"fmt"
	"testing"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
)

var mergeAccountingSink int64

// One operation is an entire aggregation window, not an individual push.
// Input cloning is outside the timer: Merge rewrites its input, so every
// window needs fresh profiles. All variants perform the same final Profile.
// This isolates merging/accounting, excluding admission control and transport.
func BenchmarkMergeAccounting(b *testing.B) {
	for _, file := range []string{"go.cpu.labels.pprof", "heap", "profile_java"} {
		raw, err := OpenFile("testdata/" + file)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(file, func(b *testing.B) {
			for _, growth := range []bool{false, true} {
				workload := "repeated"
				if growth {
					workload = "unique-label"
				}
				b.Run(workload, func(b *testing.B) {
					for _, count := range []int{1, 10, 100} {
						templates := make([]*profilev1.Profile, count)
						var summed int64
						for i := range templates {
							p := raw.Profile.CloneVT()
							if growth {
								key := int64(len(p.StringTable))
								p.StringTable = append(p.StringTable, "benchmark_contributor", fmt.Sprintf("contributor-%d", i))
								for _, s := range p.Sample {
									s.Label = append(s.Label, &profilev1.Label{Key: key, Str: key + 1})
								}
							}
							summed += int64(p.SizeVT())
							templates[i] = p
						}
						b.Run(fmt.Sprintf("n=%d", count), func(b *testing.B) {
							for _, every := range []int{0, 1, 10} {
								mode := "sum-inputs"
								if every == 1 {
									mode = "size-each-merge"
								} else if every == 10 {
									mode = "size-every-10"
								}
								b.Run(mode, func(b *testing.B) {
									b.ReportAllocs()
									var finalSize, finalReserved int64
									for i := 0; i < b.N; i++ {
										b.StopTimer()
										inputs := make([]*profilev1.Profile, count)
										sizes := make([]int64, count)
										for j, p := range templates {
											inputs[j] = p.CloneVT()
											sizes[j] = int64(p.SizeVT())
										}
										b.StartTimer()
										var m ProfileMerge
										var reserved int64
										for j, p := range inputs {
											if err := m.Merge(p, true); err != nil {
												b.Fatal(err)
											}
											reserved += sizes[j]
											if every > 0 && (j+1)%every == 0 {
												reserved = int64(m.Profile().SizeVT())
											}
										}
										result := m.Profile()
										mergeAccountingSink = reserved
										finalReserved = reserved
										b.StopTimer()
										finalSize = int64(result.SizeVT())
										b.StartTimer()
									}
									b.ReportMetric(float64(summed), "input-bytes/window")
									b.ReportMetric(float64(finalSize), "aggregate-bytes/window")
									b.ReportMetric(float64(finalReserved)/float64(finalSize), "final-reserved-x")
								})
							}
						})
					}
				})
			}
		})
	}
}

// Verify intermediate materialization doesn't change the final merged profile.
func TestMergeAccountingIntermediateProfile(t *testing.T) {
	for _, file := range []string{"go.cpu.labels.pprof", "heap", "profile_java"} {
		t.Run(file, func(t *testing.T) {
			raw, err := OpenFile("testdata/" + file)
			if err != nil {
				t.Fatal(err)
			}
			var baseline, precise ProfileMerge
			var previousSamples int
			for i := 0; i < 10; i++ {
				input := raw.Profile.CloneVT()
				key := int64(len(input.StringTable))
				input.StringTable = append(input.StringTable, "benchmark_contributor", fmt.Sprintf("contributor-%d", i))
				for _, s := range input.Sample {
					s.Label = append(s.Label, &profilev1.Label{Key: key, Str: key + 1})
				}
				if err := baseline.Merge(input.CloneVT(), true); err != nil {
					t.Fatal(err)
				}
				if err := precise.Merge(input.CloneVT(), true); err != nil {
					t.Fatal(err)
				}
				p := precise.Profile()
				_ = p.SizeVT()
				if len(p.Sample) <= previousSamples {
					t.Fatal("unique-label workload must grow on every contribution")
				}
				previousSamples = len(p.Sample)
			}
			if !baseline.Profile().EqualVT(precise.Profile()) {
				t.Fatal("intermediate Profile changed merge result")
			}
		})
	}
}
