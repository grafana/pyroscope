package flamebearer

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

var (
	startTime     = int64(1635508310)
	durationDelta = int64(10)
	samples       = []uint64{1}
	watermarks    = map[int]int64{1: 1}
	maxNodes      = 1024
	spyName       = "spy-name"
	sampleRate    = uint32(10)
	units         = "units"
)

var _ = Describe("FlamebearerProfile", func() {
	Context("single", func() {
		It("sets all attributes correctly", func() {
			// taken from tree package tests
			tree := tree.New()
			tree.Insert([]byte("a;b"), uint64(1))
			tree.Insert([]byte("a;c"), uint64(2))

			timeline := &segment.Timeline{
				StartTime:               startTime,
				Samples:                 samples,
				DurationDeltaNormalized: durationDelta,
				Watermarks:              watermarks,
			}

			out := &storage.GetOutput{
				Tree:       tree,
				Timeline:   timeline,
				SpyName:    spyName,
				SampleRate: sampleRate,
				Units:      units,
			}
			p := NewProfile(out, maxNodes)

			// Flamebearer
			Expect(p.Flamebearer.Names).To(ConsistOf("total", "a", "b", "c"))
			Expect(p.Flamebearer.Levels).To(Equal([][]int{
				{0, 3, 0, 0},
				{0, 3, 0, 1},
				{0, 1, 1, 3, 0, 2, 2, 2},
			}))
			Expect(p.Flamebearer.NumTicks).To(Equal(3))
			Expect(p.Flamebearer.MaxSelf).To(Equal(2))

			// Metadata
			Expect(p.Metadata.Format).To(Equal("single"))
			Expect(p.Metadata.SpyName).To(Equal(spyName))
			Expect(p.Metadata.SampleRate).To(Equal(sampleRate))
			Expect(p.Metadata.Units).To(Equal(units))

			// Timeline
			Expect(p.Timeline.StartTime).To(Equal(startTime))
			Expect(p.Timeline.Samples).To(Equal(samples))
			Expect(p.Timeline.DurationDelta).To(Equal(durationDelta))
			Expect(p.Timeline.Watermarks).To(Equal(watermarks))

			// Ticks
			Expect(p.LeftTicks).To(BeZero())
			Expect(p.RightTicks).To(BeZero())
		})
	})

	Context("diff", func() {
		It("sets all attributes correctly", func() {
			// taken from tree package tests
			treeA := tree.New()
			treeA.Insert([]byte("a;b"), uint64(1))
			treeA.Insert([]byte("a;c"), uint64(2))
			treeB := tree.New()
			treeB.Insert([]byte("a;b"), uint64(4))
			treeB.Insert([]byte("a;c"), uint64(8))

			timeline := &segment.Timeline{
				StartTime:               startTime,
				Samples:                 samples,
				DurationDeltaNormalized: durationDelta,
				Watermarks:              watermarks,
			}

			out := &storage.GetOutput{
				Timeline:   timeline,
				SpyName:    spyName,
				SampleRate: sampleRate,
				Units:      units,
			}
			left := &storage.GetOutput{Tree: treeA}
			right := &storage.GetOutput{Tree: treeB}
			p := NewCombinedProfile(out, left, right, maxNodes)

			// Flamebearer
			Expect(p.Flamebearer.Names).To(ConsistOf("total", "a", "b", "c"))
			Expect(p.Flamebearer.Levels).To(Equal([][]int{
				{0, 3, 0, 0, 12, 0, 0},
				{0, 3, 0, 0, 12, 0, 1},
				{0, 1, 1, 0, 4, 4, 3, 0, 2, 2, 0, 8, 8, 2},
			}))
			Expect(p.Flamebearer.NumTicks).To(Equal(15))
			Expect(p.Flamebearer.MaxSelf).To(Equal(8))

			// Metadata
			Expect(p.Metadata.Format).To(Equal("double"))
			Expect(p.Metadata.SpyName).To(Equal(spyName))
			Expect(p.Metadata.SampleRate).To(Equal(sampleRate))
			Expect(p.Metadata.Units).To(Equal(units))

			// Timeline
			Expect(p.Timeline.StartTime).To(Equal(startTime))
			Expect(p.Timeline.Samples).To(Equal(samples))
			Expect(p.Timeline.DurationDelta).To(Equal(durationDelta))
			Expect(p.Timeline.Watermarks).To(Equal(watermarks))

			// Ticks
			Expect(p.LeftTicks).To(Equal(uint64(3)))
			Expect(p.RightTicks).To(Equal(uint64(12)))
		})
	})
})
