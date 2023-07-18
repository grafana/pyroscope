package segment

import (
	"bufio"
	"log"
	"math/big"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/grafana/pyroscope/pkg/og/testing"
)

var putNoOp = func(depth int, t time.Time, r *big.Rat, addons []Addon) {}

func doGet(s *Segment, st, et time.Time) []time.Time {
	res := []time.Time{}
	s.Get(st, et, func(d int, samples, writes uint64, t time.Time, r *big.Rat) {
		res = append(res, t)
	})
	return res
}

func strip(val string) string {
	ret := ""
	scanner := bufio.NewScanner(strings.NewReader(val))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) > 0 {
			ret += line + "\n"
		}
	}
	return ret
}

func expectChildrenSamplesAddUpToParentSamples(tn *streeNode) {
	childrenSum := uint64(0)
	if len(tn.children) == 0 {
		return
	}
	for _, v := range tn.children {
		if v != nil {
			expectChildrenSamplesAddUpToParentSamples(v)
			childrenSum += v.samples
		}
	}
	Expect(childrenSum).To(Equal(tn.samples))
}

var _ = Describe("stree", func() {
	Context("Get", func() {
		Context("When there's no root", func() {
			It("get doesn't fail", func() {
				s := New()
				Expect(doGet(s, testing.SimpleTime(0), testing.SimpleTime(39))).To(HaveLen(0))
			})
		})
	})

	Context("StartTime", func() {
		Context("empty segment", func() {
			It("returns zero time", func() {
				s := New()
				Expect(s.StartTime().IsZero()).To(BeTrue())
			})
		})

		Context("fuzz test", func() {
			It("always returns the right values", func() {
				r := rand.New(rand.NewSource(6231912))

				// doesn't work with minTime = 0
				minTime := 1023886146
				maxTime := 1623886146

				runs := 100
				maxInsertionsPerTree := 100

				for i := 0; i < runs; i++ {
					s := New()
					minSt := maxTime
					for j := 0; j < 1+r.Intn(maxInsertionsPerTree); j++ {
						st := (minTime + r.Intn(maxTime-minTime)) / 10 * 10
						if st < minSt {
							minSt = st
						}
						et := st + 10 + r.Intn(1000)
						s.Put(testing.SimpleTime(st), testing.SimpleTime(et), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
					}

					Expect(s.StartTime()).To(Equal(testing.SimpleTime(minSt)))
				}
			})
		})
	})

	Context("DeleteDataBefore", func() {
		Context("empty segment", func() {
			It("returns true and no keys", func() {
				s := New()

				keys := []string{}
				rp := &RetentionPolicy{AbsoluteTime: testing.SimpleTime(19)}
				r, _ := s.WalkNodesToDelete(rp, func(depth int, t time.Time) error {
					keys = append(keys, strconv.Itoa(depth)+":"+strconv.Itoa(int(t.Unix())))
					return nil
				})

				Expect(r).To(BeTrue())
				Expect(keys).To(BeEmpty())
			})
		})

		Context("simple test 1", func() {
			It("correctly deletes data", func() {
				s := New()
				s.Put(testing.SimpleUTime(10), testing.SimpleUTime(19), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				s.Put(testing.SimpleUTime(20), testing.SimpleUTime(29), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})

				keys := []string{}
				rp := &RetentionPolicy{AbsoluteTime: testing.SimpleUTime(21)}
				r, _ := s.WalkNodesToDelete(rp, func(depth int, t time.Time) error {
					keys = append(keys, strconv.Itoa(depth)+":"+strconv.Itoa(int(t.Unix())))
					return nil
				})

				Expect(r).To(BeFalse())
				Expect(keys).To(ConsistOf([]string{
					"0:10",
				}))
			})
		})

		Context("simple test 3", func() {
			It("correctly deletes data", func() {
				s := New()
				s.Put(testing.SimpleUTime(10), testing.SimpleUTime(19), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				s.Put(testing.SimpleUTime(1020), testing.SimpleUTime(1029), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})

				keys := []string{}
				rp := &RetentionPolicy{AbsoluteTime: testing.SimpleUTime(21)}
				r, _ := s.WalkNodesToDelete(rp, func(depth int, t time.Time) error {
					keys = append(keys, strconv.Itoa(depth)+":"+strconv.Itoa(int(t.Unix())))
					return nil
				})

				Expect(r).To(BeFalse())
				Expect(keys).To(ConsistOf([]string{
					"0:10",
				}))
			})
		})

		Context("simple test 2", func() {
			It("correctly deletes data", func() {
				s := New()
				s.Put(testing.SimpleUTime(10), testing.SimpleUTime(19), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				s.Put(testing.SimpleUTime(20), testing.SimpleUTime(29), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})

				keys := []string{}
				rp := &RetentionPolicy{AbsoluteTime: testing.SimpleUTime(200)}
				r, _ := s.WalkNodesToDelete(rp, func(depth int, t time.Time) error {
					keys = append(keys, strconv.Itoa(depth)+":"+strconv.Itoa(int(t.Unix())))
					return nil
				})

				Expect(r).To(BeTrue())
				Expect(keys).To(ConsistOf([]string{
					"1:0",
					"0:10",
					"0:20",
				}))
			})
		})

		Context("level-based retention", func() {
			It("correctly deletes data partially", func() {
				s := New()
				s.Put(testing.SimpleUTime(10), testing.SimpleUTime(19), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				s.Put(testing.SimpleUTime(20), testing.SimpleUTime(29), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})

				keys := []string{}
				rp := &RetentionPolicy{Levels: map[int]time.Time{0: time.Now()}}
				r, _ := s.WalkNodesToDelete(rp, func(depth int, t time.Time) error {
					keys = append(keys, strconv.Itoa(depth)+":"+strconv.Itoa(int(t.Unix())))
					return nil
				})

				Expect(r).To(BeFalse())
				Expect(s.root).ToNot(BeNil())
				Expect(keys).To(ConsistOf([]string{
					"0:10",
					"0:20",
				}))

				removed, err := s.DeleteNodesBefore(rp)
				Expect(err).ToNot(HaveOccurred())
				Expect(removed).To(BeFalse())
			})

			It("correctly deletes data completely", func() {
				s := New()
				s.Put(testing.SimpleUTime(10), testing.SimpleUTime(19), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				s.Put(testing.SimpleUTime(20), testing.SimpleUTime(29), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})

				var keys []string
				rp := &RetentionPolicy{Levels: map[int]time.Time{0: time.Now(), 1: time.Now()}}
				r, _ := s.WalkNodesToDelete(rp, func(depth int, t time.Time) error {
					keys = append(keys, strconv.Itoa(depth)+":"+strconv.Itoa(int(t.Unix())))
					return nil
				})

				Expect(r).To(BeTrue())
				Expect(keys).To(ConsistOf([]string{
					"1:0",
					"0:10",
					"0:20",
				}))

				removed, err := s.DeleteNodesBefore(rp)
				Expect(err).ToNot(HaveOccurred())
				Expect(removed).To(BeTrue())
			})

			Context("Issue 715", func() {
				// See https://github.com/pyroscope-io/pyroscope/issues/715
				It("does not return nodes affected by retention policy", func() {
					b, err := os.Open("testdata/issue_715")
					Expect(err).ToNot(HaveOccurred())
					s, err := Deserialize(b)
					Expect(err).ToNot(HaveOccurred())

					var keys []string
					st := time.Date(2022, time.January, 12, 9, 40, 0, 0, time.UTC)
					et := time.Date(2022, time.January, 12, 10, 40, 0, 0, time.UTC)
					s.Get(st, et, func(depth int, samples, writes uint64, t time.Time, r *big.Rat) {
						keys = append(keys, strconv.Itoa(depth)+":"+strconv.Itoa(int(t.Unix()))+":"+r.String())
					})

					Expect(keys).To(BeEmpty())
				})

				It("correctly samples data", func() {
					s := New()
					st := time.Date(2021, time.December, 1, 0, 0, 0, 0, time.UTC)
					et := time.Date(2022, time.January, 1, 0, 0, 0, 0, time.UTC)
					rp := &RetentionPolicy{AbsoluteTime: et}

					c := st
					for c.Before(et) {
						e := c.Add(time.Second * time.Duration(10))
						err := s.Put(c, e, 100, func(int, time.Time, *big.Rat, []Addon) {})
						Expect(err).ToNot(HaveOccurred())
						c = e
					}

					r, err := s.DeleteNodesBefore(rp)
					Expect(r).To(BeFalse())
					Expect(err).ToNot(HaveOccurred())

					gSt := st.Add(-time.Hour)
					gEt := et.Add(time.Hour)

					var keys []string
					s.Get(gSt, gEt, func(depth int, samples, writes uint64, t time.Time, r *big.Rat) {
						keys = append(keys, strconv.Itoa(depth)+":"+strconv.Itoa(int(t.Unix()))+":"+r.String())
					})

					Expect(keys).To(BeEmpty())
				})

				It("correctly samples data with level retention period", func() {
					s := New()
					st := time.Date(2021, time.December, 1, 0, 0, 0, 0, time.UTC)
					et := time.Date(2021, time.December, 2, 0, 0, 0, 0, time.UTC)

					c := st
					for c.Before(et) {
						e := c.Add(time.Second * time.Duration(10))
						err := s.Put(c, e, 100, func(int, time.Time, *big.Rat, []Addon) {})
						Expect(err).ToNot(HaveOccurred())
						c = e
					}

					r, err := s.DeleteNodesBefore(&RetentionPolicy{Levels: map[int]time.Time{0: et}})
					Expect(r).To(BeFalse())
					Expect(err).ToNot(HaveOccurred())

					gSt := time.Date(2021, time.December, 1, 10, 0, 0, 0, time.UTC)
					gEt := gSt.Add(time.Second * 30)

					var keys []string
					s.Get(gSt, gEt, func(depth int, samples, writes uint64, t time.Time, r *big.Rat) {
						keys = append(keys, strconv.Itoa(depth)+":"+strconv.Itoa(int(t.Unix()))+":"+r.String())
					})

					Expect(keys).To(ConsistOf([]string{
						"1:1638352800:3/10",
					}))
				})
			})
		})
	})

	Context("Put", func() {
		Context("When inserts are far apart", func() {
			Context("When second insert is far in the future", func() {
				It("sets root properly", func() {
					log.Println("---")
					s := New()
					s.Put(testing.SimpleTime(1330),
						testing.SimpleTime(1339), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
					Expect(s.root).ToNot(BeNil())
					Expect(s.root.depth).To(Equal(0))
					s.Put(testing.SimpleTime(1110),
						testing.SimpleTime(1119), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
					expectChildrenSamplesAddUpToParentSamples(s.root)
				})
			})
			Context("When second insert is far in the past", func() {
				It("sets root properly", func() {
					log.Println("---")
					s := New()
					s.Put(testing.SimpleTime(2030),
						testing.SimpleTime(2039), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
					Expect(s.root).ToNot(BeNil())
					Expect(s.root.depth).To(Equal(0))
					s.Put(testing.SimpleTime(0),
						testing.SimpleTime(9), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
					expectChildrenSamplesAddUpToParentSamples(s.root)
				})
			})
		})

		Context("When empty", func() {
			It("sets root properly", func() {
				s := New()
				s.Put(testing.SimpleTime(0),
					testing.SimpleTime(9), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(0))
			})

			It("sets root properly", func() {
				s := New()
				s.Put(testing.SimpleTime(0),
					testing.SimpleTime(49), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(1))
			})

			It("sets root properly", func() {
				s := New()
				s.Put(testing.SimpleTime(10),
					testing.SimpleTime(109), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(2))
			})

			It("sets root properly", func() {
				s := New()
				s.Put(testing.SimpleTime(10),
					testing.SimpleTime(19), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(0))
				s.Put(testing.SimpleTime(10),
					testing.SimpleTime(19), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				expectChildrenSamplesAddUpToParentSamples(s.root)
			})

			It("sets root properly", func() {
				s := New()
				s.Put(testing.SimpleTime(10),
					testing.SimpleTime(19), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(0))
				s.Put(testing.SimpleTime(20),
					testing.SimpleTime(29), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(1))
				expectChildrenSamplesAddUpToParentSamples(s.root)
			})

			It("sets root properly", func() {
				s := New()
				s.Put(testing.SimpleTime(10),
					testing.SimpleTime(19), 10, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(0))
				s.Put(testing.SimpleTime(20),
					testing.SimpleTime(39), 10, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(1))
				expectChildrenSamplesAddUpToParentSamples(s.root)
			})

			It("sets root properly", func() {
				s := New()
				s.Put(testing.SimpleTime(10),
					testing.SimpleTime(19), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(0))

				s.Put(testing.SimpleTime(20),
					testing.SimpleTime(29), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(1))

				s.Put(testing.SimpleTime(30),
					testing.SimpleTime(39), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(1))
				expectChildrenSamplesAddUpToParentSamples(s.root)
			})

			It("sets root properly", func() {
				s := New()
				s.Put(testing.SimpleTime(30),
					testing.SimpleTime(39), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(0))

				s.Put(testing.SimpleTime(20),
					testing.SimpleTime(29), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(1))

				s.Put(testing.SimpleTime(10),
					testing.SimpleTime(19), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(1))

				Expect(doGet(s, testing.SimpleTime(0), testing.SimpleTime(39))).To(HaveLen(3))
			})

			It("works with 3 mins", func() {
				s := New()
				s.Put(testing.SimpleTime(10),
					testing.SimpleTime(70), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(1))
				// Expect(doGet(s, testing.SimpleTime(20, testing.SimpleTime(49))).To(HaveLen(3))
			})

			It("sets trie properly, gets work", func() {
				s := New()

				s.Put(testing.SimpleTime(0),
					testing.SimpleTime(9), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(0))

				s.Put(testing.SimpleTime(100),
					testing.SimpleTime(109), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				expectChildrenSamplesAddUpToParentSamples(s.root)
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(2))
				Expect(s.root.present).To(BeTrue())
				Expect(s.root.children[0]).ToNot(BeNil())
				Expect(s.root.children[0].present).ToNot(BeTrue())
				Expect(s.root.children[1]).ToNot(BeNil())
				Expect(s.root.children[1].present).ToNot(BeTrue())
				Expect(s.root.children[0].children[0].present).To(BeTrue())
				Expect(s.root.children[1].children[0].present).To(BeTrue())

				Expect(doGet(s, testing.SimpleTime(0), testing.SimpleTime(9))).To(HaveLen(1))
				Expect(doGet(s, testing.SimpleTime(10), testing.SimpleTime(19))).To(HaveLen(0))
				Expect(doGet(s, testing.SimpleTime(100), testing.SimpleTime(109))).To(HaveLen(1))
				Expect(doGet(s, testing.SimpleTime(0), testing.SimpleTime(109))).To(HaveLen(2))
				Expect(doGet(s, testing.SimpleTime(0), testing.SimpleTime(999))).To(HaveLen(1))
				Expect(doGet(s, testing.SimpleTime(0), testing.SimpleTime(1000))).To(HaveLen(1))
				Expect(doGet(s, testing.SimpleTime(0), testing.SimpleTime(1001))).To(HaveLen(1))
				Expect(doGet(s, testing.SimpleTime(0), testing.SimpleTime(989))).To(HaveLen(2))
			})
		})
	})
})
