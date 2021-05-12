package segment

import (
	"bufio"
	"log"
	"math/big"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

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
