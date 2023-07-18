package tree

import (
	"time"

	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
)

var _ = Describe("tree", func() {
	var tree *Tree
	var profile *Profile
	var fnNames []string

	Describe("pprof", func() {
		BeforeEach(func() {
			tree = New()
			tree.Insert([]byte("main;foo;1;2;3;4;5"), uint64(15))
			tree.Insert([]byte("main;baz;1;2;3;4;5;6"), uint64(55))
			tree.Insert([]byte("main;baz;1;2;3;4;5;6;8"), uint64(25))
			tree.Insert([]byte("main;bar;1;2;3;4;5;6;8"), uint64(35))
			tree.Insert([]byte("main;foo;1;2;3;4;5;6"), uint64(20))
			tree.Insert([]byte("main;bar;1;2;3;4;5;6;9"), uint64(35))
			tree.Insert([]byte("main;baz;1;2;3;4;5;6;7"), uint64(30))
			tree.Insert([]byte("main;qux;1;2;3;4;5;6;7;8;9"), uint64(65))

			profile = tree.Pprof(&PprofMetadata{
				Type:      "cpu",
				Unit:      "samples",
				StartTime: time.Date(2021, 1, 1, 10, 0, 1, 0, time.UTC),
				Duration:  time.Minute,
			})
			fnNames = []string{
				"main",
				"foo",
				"bar",
				"baz",
				"qux",
				"1",
				"2",
				"3",
				"4",
				"5",
				"6",
				"7",
				"8",
				"9",
			}
		})
		It("Should serialize correctly", func() {
			_, err := proto.Marshal(profile)
			Expect(err).ToNot(HaveOccurred())
		})
		Describe("StringTable", func() {
			It("Should build correctly", func() {
				Expect(len(profile.StringTable)).To(Equal(17))
				Expect(profile.StringTable).To(ConsistOf(
					append(
						fnNames,
						"",
						"cpu",
						"samples",
					),
				))
			})
			It("Should have empty first element", func() {
				profile = tree.Pprof(&PprofMetadata{})
				Expect(profile.StringTable[0]).To(Equal(""))
			})
		})
		Describe("Metadata", func() {
			It("Should build correctly", func() {
				Expect(profile.TimeNanos).To(Equal(int64(1609495201000000000)))
				Expect(profile.DurationNanos).To(Equal(int64(60000000000)))
				Expect(len(profile.SampleType)).To(Equal(1))
				_type := profile.StringTable[profile.SampleType[0].Type]
				unit := profile.StringTable[profile.SampleType[0].Unit]

				Expect(_type).To(Equal("cpu"))
				Expect(unit).To(Equal("samples"))
			})
		})
		Describe("Function", func() {
			It("Should build correctly", func() {
				Expect(len(profile.Function)).To(Equal(14))
				for _, fn := range profile.Function {
					Expect(fn.Id).NotTo(BeZero())
					Expect(fn.Name).NotTo(BeZero())
					Expect(fn.SystemName).NotTo(BeZero())
				}
			})
			Context("Name", func() {
				It("Should have corresponding StringTable entries", func() {
					var names []string
					for _, fn := range profile.Function {
						fnName := profile.StringTable[fn.Name]
						names = append(names, fnName)
					}
					Expect(names).To(ConsistOf(fnNames))
				})
			})
			Context("SystemName", func() {
				It("Should have corresponding StringTable entries", func() {
					var names []string
					for _, fn := range profile.Function {
						fnName := profile.StringTable[fn.SystemName]
						names = append(names, fnName)
					}
					Expect(names).To(ConsistOf(fnNames))
				})
			})
		})
		Describe("Location", func() {
			It("Should build correctly", func() {
				Expect(len(profile.Location)).To(Equal(14))
				for _, l := range profile.Location {
					Expect(l.Id).NotTo(BeZero())
					Expect(l.Line).NotTo(BeZero())
					Expect(l.Line[0].FunctionId).NotTo(BeZero())
				}
			})
			It("Should have corresponding functions", func() {
				fnMap := make(map[uint64]*Function)
				for _, fn := range profile.Function {
					fnMap[fn.Id] = fn
				}
				for _, l := range profile.Location {
					fnID := l.Line[0].FunctionId
					_, ok := fnMap[fnID]
					Expect(ok).To(BeTrue())
				}
			})
		})
		Describe("Sample", func() {
			It("Should build correctly", func() {
				Expect(len(profile.Sample)).To(Equal(8))
				for _, s := range profile.Sample {
					Expect(s.LocationId).NotTo(BeZero())
					Expect(s.Value).NotTo(BeZero())
				}
			})
			It("Should have corresponding locations", func() {
				lMap := make(map[uint64]*Location)
				for _, l := range profile.Location {
					lMap[l.Id] = l
				}
				for _, s := range profile.Sample {
					for _, l := range s.LocationId {
						_, ok := lMap[l]
						Expect(ok).To(BeTrue())
					}
				}
			})
		})
	})
})
