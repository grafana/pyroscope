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

		cache := New(db, "prefix:")
		cache.Bytes = func(k string, v interface{}) []byte {
			return []byte(v.(string))
		}
		cache.FromBytes = func(k string, v []byte) interface{} {
			return string(v)
		}
		for i := 0; i < 200; i++ {
			cache.Put(fmt.Sprintf("foo-%d", i), fmt.Sprintf("bar-%d", i))
		}
		log.Printf("size: %d", cache.Len())

		Expect(cache.Get("foo-199")).To(Equal("bar-199"))
		Expect(cache.Get("foo-1")).To(Equal("bar-1"))
		Expect(cache.Get("foo-1234")).To(BeNil())
		cache.Flush()

		close(done)
	}, 3)
})
