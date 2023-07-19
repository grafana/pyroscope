package flamebearer

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/grafana/pyroscope/pkg/og/storage/metadata"
	"github.com/grafana/pyroscope/pkg/og/storage/segment"
	"github.com/grafana/pyroscope/pkg/og/storage/tree"
)

var (
	startTime     = int64(1635508310)
	durationDelta = int64(10)
	samples       = []uint64{1}
	watermarks    = map[int]int64{1: 1}
	maxNodes      = 1024
	spyName       = "spy-name"
	sampleRate    = uint32(10)
	units         = metadata.Units("units")
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

			p := NewProfile(ProfileConfig{
				Name:     "name",
				Tree:     tree,
				MaxNodes: maxNodes,
				Timeline: timeline,
				Metadata: metadata.Metadata{
					SpyName:    spyName,
					SampleRate: sampleRate,
					Units:      units,
				},
			})

			// Flamebearer
			Expect(p.Flamebearer.Names).To(Equal([]string{"total", "a", "c", "b"}))
			Expect(p.Flamebearer.Levels).To(Equal([][]int{
				{0, 3, 0, 0},
				{0, 3, 0, 1},
				{0, 1, 1, 3, 0, 2, 2, 2},
			}))
			Expect(p.Flamebearer.NumTicks).To(Equal(3))
			Expect(p.Flamebearer.MaxSelf).To(Equal(2))

			// Metadata
			Expect(p.Metadata.Name).To(Equal("name"))
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

			// Validate
			Expect(p.Validate()).To(BeNil())
		})
	})
})

var _ = Describe("Diff", func() {
	var treeA *tree.Tree
	var treeB *tree.Tree

	BeforeEach(func() {
		treeA = tree.New()
		treeA.Insert([]byte("a;b"), uint64(1))
		treeA.Insert([]byte("a;c"), uint64(2))
		treeB = tree.New()
		treeB.Insert([]byte("a;b"), uint64(4))
		treeB.Insert([]byte("a;c"), uint64(8))
	})

	Context("sampleRate does not match", func() {
		When("they are both set", func() {
			It("returns an error", func() {
				left := ProfileConfig{
					Name:     "name",
					Metadata: metadata.Metadata{SampleRate: 100},
					Tree:     treeA,
					MaxNodes: maxNodes,
				}
				right := ProfileConfig{
					Name:     "name",
					Metadata: metadata.Metadata{SampleRate: 101},
					Tree:     treeB,
					MaxNodes: maxNodes,
				}
				_, err := NewCombinedProfile(left, right)
				Expect(err).To(MatchError("left sample rate (100) does not match right sample rate (101)"))
			})
		})

		When("one of them is empty", func() {
			It("does not return an error", func() {
				left := ProfileConfig{
					Name:     "name",
					Metadata: metadata.Metadata{SampleRate: 0},
					Tree:     treeA,
					MaxNodes: maxNodes,
				}
				right := ProfileConfig{
					Name:     "name",
					Metadata: metadata.Metadata{SampleRate: 101},
					Tree:     treeB,
					MaxNodes: maxNodes,
				}

				_, err := NewCombinedProfile(left, right)
				Expect(err).ToNot(HaveOccurred())

				_, err = NewCombinedProfile(right, left)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Context("units does not match", func() {
		When("they are both set", func() {
			It("returns an error", func() {
				left := ProfileConfig{
					Name:     "name",
					Metadata: metadata.Metadata{Units: "unitA", SampleRate: sampleRate},
					Tree:     treeA,
					MaxNodes: maxNodes,
				}
				right := ProfileConfig{
					Name:     "name",
					Metadata: metadata.Metadata{Units: "unitB", SampleRate: sampleRate},
					Tree:     treeB,
					MaxNodes: maxNodes,
				}
				_, err := NewCombinedProfile(left, right)
				Expect(err).To(MatchError("left units (unitA) does not match right units (unitB)"))
			})
		})

		When("one of them is empty", func() {
			It("does not return an error", func() {
				left := ProfileConfig{
					Name:     "name",
					Metadata: metadata.Metadata{SampleRate: sampleRate},
					Tree:     treeA,
					MaxNodes: maxNodes,
				}
				right := ProfileConfig{
					Name:     "name",
					Metadata: metadata.Metadata{SampleRate: sampleRate, Units: "unitB"},
					Tree:     treeB,
					MaxNodes: maxNodes,
				}

				_, err := NewCombinedProfile(left, right)
				Expect(err).ToNot(HaveOccurred())

				_, err = NewCombinedProfile(right, left)
				Expect(err).ToNot(HaveOccurred())
			})
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

			left := ProfileConfig{
				Name:     "name",
				Metadata: metadata.Metadata{SampleRate: sampleRate, Units: units},
				Tree:     treeA,
				MaxNodes: maxNodes,
			}
			right := ProfileConfig{
				Name:     "name",
				Metadata: metadata.Metadata{SampleRate: sampleRate, Units: units},
				Tree:     treeB,
				MaxNodes: maxNodes,
			}

			p, err := NewCombinedProfile(left, right)
			Expect(err).ToNot(HaveOccurred())

			// Flamebearer
			Expect(p.Flamebearer.Names).To(Equal([]string{"total", "a", "c", "b"}))
			Expect(p.Flamebearer.Levels).To(Equal([][]int{
				{0, 3, 0, 0, 12, 0, 0},
				{0, 3, 0, 0, 12, 0, 1},
				{0, 1, 1, 0, 4, 4, 3, 0, 2, 2, 0, 8, 8, 2},
			}))
			Expect(p.Flamebearer.NumTicks).To(Equal(15))
			Expect(p.Flamebearer.MaxSelf).To(Equal(8))

			// Metadata
			Expect(p.Metadata.Name).To(Equal("name"))
			Expect(p.Metadata.Format).To(Equal("double"))
			Expect(p.Metadata.SpyName).To(Equal(""))
			Expect(p.Metadata.SampleRate).To(Equal(sampleRate))
			Expect(p.Metadata.Units).To(Equal(units))

			// Timeline
			Expect(p.Timeline).To(BeNil())

			// Ticks
			Expect(p.LeftTicks).To(Equal(uint64(3)))
			Expect(p.RightTicks).To(Equal(uint64(12)))

			// Validate
			Expect(p.Validate()).To(BeNil())
		})
	})
})

var _ = Describe("Checking profile validation", func() {
	When("the version is invalid", func() {
		var fb FlamebearerProfile
		BeforeEach(func() {
			fb.Version = 2
		})

		Context("and we validate the profile", func() {
			It("returns an error", func() {
				Expect(fb.Validate()).To(MatchError("unsupported version 2"))
			})
		})
	})

	When("the format is unsupported", func() {
		var fb FlamebearerProfile

		Context("and we validate the profile", func() {
			It("returns an error", func() {
				Expect(fb.Validate()).To(MatchError("unsupported format "))
			})
		})
	})

	When("there are no names", func() {
		var fb FlamebearerProfile
		BeforeEach(func() {
			fb.Metadata.Format = "single"
		})

		Context("and we validate the profile", func() {
			It("returns an error", func() {
				Expect(fb.Validate()).To(MatchError("a profile must have at least one symbol name"))
			})
		})
	})

	When("there are no levels", func() {
		var fb FlamebearerProfile
		BeforeEach(func() {
			fb.Metadata.Format = "single"
			fb.Flamebearer.Names = []string{"name"}
		})

		Context("and we validate the profile", func() {
			It("returns an error", func() {
				Expect(fb.Validate()).To(MatchError("a profile must have at least one profiling level"))
			})
		})
	})

	When("the level size is invalid for the profile format", func() {
		Context("and we validate a single profile", func() {
			var fb FlamebearerProfile
			BeforeEach(func() {
				fb.Metadata.Format = "single"
				fb.Flamebearer.Names = []string{"name"}
				fb.Flamebearer.Levels = [][]int{{0, 0, 0, 0, 0, 0, 0}}
			})

			It("returns an error", func() {
				Expect(fb.Validate()).To(MatchError("a profile level should have a multiple of 4 values, but there's a level with 7 values"))
			})
		})

		Context("and we validate a double profile", func() {
			var fb FlamebearerProfile
			BeforeEach(func() {
				fb.Metadata.Format = "double"
				fb.Flamebearer.Names = []string{"name"}
				fb.Flamebearer.Levels = [][]int{{0, 0, 0, 0}}
			})

			It("returns an error", func() {
				Expect(fb.Validate()).To(MatchError("a profile level should have a multiple of 7 values, but there's a level with 4 values"))
			})
		})
	})

	When("the name index is invalid", func() {
		Context("and we validate a single profile", func() {
			var fb FlamebearerProfile
			BeforeEach(func() {
				fb.Metadata.Format = "single"
				fb.Flamebearer.Names = []string{"name"}
				fb.Flamebearer.Levels = [][]int{{0, 0, 0, 1}}
			})

			It("returns an error", func() {
				Expect(fb.Validate()).To(MatchError("invalid name index 1, it should be smaller than 1"))
			})
		})

		Context("and we validate a double profile", func() {
			var fb FlamebearerProfile
			BeforeEach(func() {
				fb.Metadata.Format = "double"
				fb.Flamebearer.Names = []string{"name"}
				fb.Flamebearer.Levels = [][]int{{0, 0, 0, 0, 0, 0, 1}}
			})

			It("returns an error", func() {
				Expect(fb.Validate()).To(MatchError("invalid name index 1, it should be smaller than 1"))
			})
		})
	})

	When("everything is valid", func() {
		Context("and we validate a single profile", func() {
			var fb FlamebearerProfile
			BeforeEach(func() {
				fb.Metadata.Format = "single"
				fb.Flamebearer.Names = []string{"name"}
				fb.Flamebearer.Levels = [][]int{{0, 0, 0, 0}}
			})

			It("returns no error", func() {
				Expect(fb.Validate()).To(BeNil())
			})
		})

		Context("and we validate a double profile", func() {
			var fb FlamebearerProfile
			BeforeEach(func() {
				fb.Metadata.Format = "double"
				fb.Flamebearer.Names = []string{"name"}
				fb.Flamebearer.Levels = [][]int{{0, 0, 0, 0, 0, 0, 0}}
			})

			It("returns an error", func() {
				Expect(fb.Validate()).To(BeNil())
			})
		})
	})
})
