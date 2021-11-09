package storage

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/config"
)

var _ = Describe("Storage config", func() {
	cfg := config.Server{
		BadgerLogLevel:        "debug",
		StoragePath:           "/var/lib/pyroscope",
		CacheEvictThreshold:   0.25,
		CacheEvictVolume:      0.33,
		BadgerNoTruncate:      true,
		MaxNodesSerialization: 2048,
		HideApplications:      []string{"app"},
		Retention:             24 * time.Hour,
		SampleRate:            100,
		CacheDimensionSize:    1,
		CacheDictionarySize:   1,
		CacheSegmentSize:      1,
		CacheTreeSize:         1,
	}
	Context("Basic config", func() {
		It("NewConfig returns storage config", func() {
			c := NewConfig(&cfg)
			Expect(c.badgerLogLevel).To(Equal("debug"))
			Expect(c.badgerNoTruncate).To(BeTrue())
			Expect(c.badgerBasePath).To(Equal("/var/lib/pyroscope"))
			Expect(c.cacheEvictThreshold).To(Equal(0.25))
			Expect(c.cacheEvictVolume).To(Equal(0.33))
			Expect(c.maxNodesSerialization).To(Equal(2048))
			Expect(c.retention).To(Equal(24 * time.Hour))
			Expect(c.hideApplications).To(HaveLen(1))
			Expect(c.hideApplications).To(ContainElement("app"))
		})

		It("WithPath returns storage config with overriden storage base path", func() {
			c := NewConfig(&cfg).WithPath("/tmp/pyroscope")
			Expect(c.badgerBasePath).To(Equal("/tmp/pyroscope"))
		})
	})
})
