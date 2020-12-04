package storage

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/petethepig/pyroscope/pkg/config"
	"github.com/petethepig/pyroscope/pkg/storage/dict"
	"github.com/petethepig/pyroscope/pkg/storage/tree"
	"github.com/petethepig/pyroscope/pkg/testing"
)

// 21:22:08      air |  (time.Duration) 10s,
// 21:22:08      air |  (time.Duration) 1m40s,
// 21:22:08      air |  (time.Duration) 16m40s,
// 21:22:08      air |  (time.Duration) 2h46m40s,
// 21:22:08      air |  (time.Duration) 27h46m40s,
// 21:22:08      air |  (time.Duration) 277h46m40s,
// 21:22:08      air |  (time.Duration) 2777h46m40s,
// 21:22:08      air |  (time.Duration) 27777h46m40s

var s *Storage
var s2 *Storage

var _ = Describe("storage package", func() {
	var tmpDir *testing.TmpDirectory
	var cfg *config.Config

	BeforeEach(func() {
		tmpDir = testing.TmpDirSync()
		cfg = config.NewForTests(tmpDir.Path)
		cfg.Server.CacheSegmentSize = 0
		cfg.Server.CacheTreeSize = 0
		var err error
		s, err = New(cfg)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		tmpDir.Close()
	})

	Context("Storage", func() {
		XIt("works", func() {
			d := dict.New()
			tree := tree.New()
			tree.Insert([]byte("a;b"), uint64(1))
			tree.Insert([]byte("a;c"), uint64(2))
			st := testing.ParseTime("0001-01-01-00:00:10")
			et := testing.ParseTime("0001-01-01-00:00:19")
			st2 := testing.ParseTime("0001-01-01-00:00:00")
			et2 := testing.ParseTime("0001-01-01-00:00:30")
			key, _ := ParseKey("foo")
			err := s.Put(st, et, key, tree)
			Expect(err).ToNot(HaveOccurred())

			t2, err := s.Get(st2, et2, key)
			Expect(err).ToNot(HaveOccurred())
			Expect(t2).ToNot(BeNil())

			Expect(t2.String(d)).To(Equal(tree.String(d)))

			Expect(s.Close()).ToNot(HaveOccurred())

			s2, err = New(cfg)

			t3, err := s2.Get(st2, et2, key)
			Expect(err).ToNot(HaveOccurred())
			Expect(t3).ToNot(BeNil())

			Expect(t3.String(d)).To(Equal(tree.String(d)))

			Expect(nil).ToNot(BeNil())
		})
	})
})
