package segment

import (
	"bufio"
	"bytes"
	"math/big"
	"strings"

	"github.com/sirupsen/logrus"

	"time"

	"github.com/davecgh/go-spew/spew"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

func doGet(s *Segment, st, et time.Time) []time.Time {
	res := []time.Time{}
	s.Get(st, et, func(d int, t time.Time, r *big.Rat) {
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

var _ = Describe("stree", func() {
	r := 10 * time.Second
	m := 10
	// var cfg *config.Config

	var tmpDir *testing.TmpDirectory

	BeforeEach(func() {
		tmpDir = testing.TmpDirSync()
		var err error
		Expect(err).ToNot(HaveOccurred())
		// cfg = config.NewForTests("/tmp")
	})
	AfterEach(func() {
		tmpDir.Close()
	})

	Context("Serialize / Deserialize", func() {
		It("returns serialized value", func() {
			s := New(r, m)
			s.Put(testing.SimpleTime(0), testing.SimpleTime(9), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
			s.Put(testing.SimpleTime(10), testing.SimpleTime(19), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
			s.Put(testing.SimpleTime(20), testing.SimpleTime(29), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})

			var buf bytes.Buffer
			s.Serialize(&buf)
			serialized := buf.Bytes()
			// spew.Dump(s.root)
			s, err := Deserialize(r, m, bytes.NewReader(serialized))
			Expect(err).ToNot(HaveOccurred())
			var buf2 bytes.Buffer
			s.Serialize(&buf2)
			serialized2 := buf2.Bytes()
			spew.Dump(s.root)
			logrus.Debugf("1: %q", serialized)
			logrus.Debugf("2: %q", serialized2)
			Expect(string(serialized2)).To(Equal(string(serialized)))
		})
	})

	Context("Put", func() {
		Context("When empty", func() {
			It("sets root properly", func() {
				s := New(r, m)
				s.Put(testing.SimpleTime(0), testing.SimpleTime(9), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(0))
			})

			It("sets root properly", func() {
				s := New(r, m)
				s.Put(testing.SimpleTime(0), testing.SimpleTime(49), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(1))
			})

			It("sets root properly", func() {
				s := New(r, m)
				s.Put(testing.SimpleTime(10), testing.SimpleTime(109), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(2))
			})

			It("sets root properly", func() {
				s := New(r, m)
				s.Put(testing.SimpleTime(10), testing.SimpleTime(19), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(0))
				s.Put(testing.SimpleTime(10), testing.SimpleTime(19), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
			})

			It("sets root properly", func() {
				s := New(r, m)
				s.Put(testing.SimpleTime(10), testing.SimpleTime(19), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(0))
				s.Put(testing.SimpleTime(20), testing.SimpleTime(29), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(1))
			})

			It("sets root properly", func() {
				s := New(r, m)

				s.Put(testing.SimpleTime(10), testing.SimpleTime(19), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(0))

				s.Put(testing.SimpleTime(20), testing.SimpleTime(29), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(1))

				s.Put(testing.SimpleTime(30), testing.SimpleTime(39), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(1))
				spew.Dump(s.root)
			})

			It("sets root properly", func() {
				s := New(r, m)

				s.Put(testing.SimpleTime(30), testing.SimpleTime(39), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(0))

				s.Put(testing.SimpleTime(20), testing.SimpleTime(29), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(1))

				s.Put(testing.SimpleTime(10), testing.SimpleTime(19), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(1))

				spew.Dump(s.root)

				Expect(doGet(s, testing.SimpleTime(0), testing.SimpleTime(39))).To(HaveLen(3))
			})

			It("works with 3 mins", func() {
				s := New(r, m)
				s.Put(testing.SimpleTime(10), testing.SimpleTime(70), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(1))
				spew.Dump(s.root)
				// Expect(doGet(s, testing.SimpleTime(20, testing.SimpleTime(49))).To(HaveLen(3))
			})

			It("sets trie properly, gets work", func() {
				s := New(r, m)

				s.Put(testing.SimpleTime(0), testing.SimpleTime(9), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				Expect(s.root).ToNot(BeNil())
				Expect(s.root.depth).To(Equal(0))
				spew.Dump(s.root)

				s.Put(testing.SimpleTime(100), testing.SimpleTime(109), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				spew.Dump(s.root)
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
