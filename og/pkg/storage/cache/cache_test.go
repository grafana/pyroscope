//go:build !windows
// +build !windows

package cache

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

type fakeCodec struct{}

const fakeCodecEmptyStub = "empty"

func (fakeCodec) New(_ string) interface{} { return fakeCodecEmptyStub }

func (fakeCodec) Serialize(w io.Writer, _ string, v interface{}) error {
	_, err := w.Write([]byte(v.(string)))
	return err
}

func (fakeCodec) Deserialize(r io.Reader, _ string) (interface{}, error) {
	b, err := io.ReadAll(r)
	return string(b), err
}

var _ = Describe("cache", func() {
	var c *Cache
	testing.WithConfig(func(cfg **config.Config) {
		JustBeforeEach(func() {
			badgerPath := filepath.Join((*cfg).Server.StoragePath)
			err := os.MkdirAll(badgerPath, 0o755)
			Expect(err).ToNot(HaveOccurred())

			badgerOptions := badger.DefaultOptions(badgerPath)
			badgerOptions = badgerOptions.WithTruncate(false)
			badgerOptions = badgerOptions.WithSyncWrites(false)
			badgerOptions = badgerOptions.WithCompression(options.ZSTD)

			db, err := badger.Open(badgerOptions)
			Expect(err).ToNot(HaveOccurred())

			reg := prometheus.NewRegistry()
			c = New(Config{
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
		})
	})

	It("works properly", func() {
		done := make(chan interface{})
		go func() {
			for i := 0; i < 200; i++ {
				c.Put(fmt.Sprintf("foo-%d", i), fmt.Sprintf("bar-%d", i))
			}

			v, err := c.GetOrCreate("foo-199")
			Expect(err).ToNot(HaveOccurred())
			Expect(v).To(Equal("bar-199"))

			v, err = c.GetOrCreate("foo-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(v).To(Equal("bar-1"))

			v, err = c.GetOrCreate("foo-1234")
			Expect(err).ToNot(HaveOccurred())
			Expect(v).To(Equal(fakeCodecEmptyStub))
			c.Flush()

			close(done)
		}()
		Eventually(done, 3).Should(BeClosed())
	})

	Context("discard prefix", func() {
		It("removes data from cache and disk", func() {
			const (
				prefixToDelete = "0:"
				prefixToKeep   = "1:"
				n              = 5 * defaultBatchSize
			)

			for i := 0; i < n; i++ {
				v := strconv.Itoa(i)
				c.Put(prefixToDelete+v, v)
				c.Put(prefixToKeep+v, v)
			}

			k := prefixToDelete + strconv.Itoa(0)
			_, ok := c.Lookup(k)
			Expect(ok).To(BeTrue())
			c.Flush()

			v, err := c.GetOrCreate(k)
			Expect(err).ToNot(HaveOccurred())
			Expect(v).To(Equal("0"))
			_, ok = c.Lookup(k)
			Expect(ok).To(BeTrue())

			Expect(c.DiscardPrefix(prefixToDelete)).ToNot(HaveOccurred())

			v, err = c.GetOrCreate(k)
			Expect(err).ToNot(HaveOccurred())
			Expect(v).To(Equal(fakeCodecEmptyStub))

			k = prefixToKeep + strconv.Itoa(0)
			v, err = c.GetOrCreate(k)
			Expect(err).ToNot(HaveOccurred())
			Expect(v).To(Equal("0"))
		})
	})
})
