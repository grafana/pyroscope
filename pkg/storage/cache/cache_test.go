//go:build !windows
// +build !windows

package cache

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

type fakeCodec struct{}

func (fakeCodec) New(k string) interface{} { return k }

func (fakeCodec) Serialize(_ io.Writer, _ string, _ interface{}) error { return nil }

func (fakeCodec) Deserialize(_ io.Reader, _ string) (interface{}, error) { return nil, nil }

var _ = Describe("cache", func() {
	It("works properly", func(done Done) {
		tdir := testing.TmpDirSync()
		badgerPath := filepath.Join(tdir.Path)
		err := os.MkdirAll(badgerPath, 0o755)
		Expect(err).ToNot(HaveOccurred())

		badgerOptions := badger.DefaultOptions(badgerPath)
		badgerOptions = badgerOptions.WithTruncate(false)
		badgerOptions = badgerOptions.WithSyncWrites(false)
		badgerOptions = badgerOptions.WithCompression(options.ZSTD)

		db, err := badger.Open(badgerOptions)
		Expect(err).ToNot(HaveOccurred())

		reg := prometheus.NewRegistry()
		cache := New(Config{
			DB:     db,
			Codec:  fakeCodec{},
			Prefix: "p:",
			Metrics: &Metrics{
				MissesCounter: promauto.With(reg).NewCounter(prometheus.CounterOpts{
					Name: "cache_test_miss",
				}),
				ReadsCounter: promauto.With(reg).NewCounter(prometheus.CounterOpts{
					Name: "storage_test_read",
				}),
				DBWrites: promauto.With(reg).NewHistogram(prometheus.HistogramOpts{
					Name: "storage_test_write",
				}),
				DBReads: promauto.With(reg).NewHistogram(prometheus.HistogramOpts{
					Name: "storage_test_reads",
				}),
			},
		})

		for i := 0; i < 200; i++ {
			cache.Put(fmt.Sprintf("foo-%d", i), fmt.Sprintf("bar-%d", i))
		}

		v, err := cache.GetOrCreate("foo-199")
		Expect(err).ToNot(HaveOccurred())
		Expect(v).To(Equal("bar-199"))

		v, err = cache.GetOrCreate("foo-1")
		Expect(err).ToNot(HaveOccurred())
		Expect(v).To(Equal("bar-1"))

		v, err = cache.GetOrCreate("foo-1234")
		Expect(err).ToNot(HaveOccurred())
		Expect(v).To(Equal("foo-1234"))
		cache.Flush()

		close(done)
	}, 3)
})
