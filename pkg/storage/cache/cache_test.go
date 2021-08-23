package cache

import (
	"fmt"
	"log"
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
		cache := New(db, "prefix:", &Metrics{
			HitCounter: promauto.With(reg).NewCounter(prometheus.CounterOpts{
				Name: "cache_test_hit",
			}),
			MissCounter: promauto.With(reg).NewCounter(prometheus.CounterOpts{
				Name: "cache_test_miss",
			}),
			ReadCounter: promauto.With(reg).NewCounter(prometheus.CounterOpts{
				Name: "storage_test_read",
			}),
			WritesToDiskCounter: promauto.With(reg).NewCounter(prometheus.CounterOpts{
				Name: "storage_test_write",
			}),
		})
		cache.New = func(k string) interface{} {
			return k
		}
		cache.Bytes = func(k string, v interface{}) ([]byte, error) {
			return []byte(v.(string)), nil
		}
		cache.Bytes = func(k string, v interface{}) ([]byte, error) {
			return []byte(v.(string)), nil
		}
		cache.FromBytes = func(k string, v []byte) (interface{}, error) {
			return string(v), nil
		}
		for i := 0; i < 200; i++ {
			cache.Put(fmt.Sprintf("foo-%d", i), fmt.Sprintf("bar-%d", i))
		}
		log.Printf("size: %d", cache.Len())

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
