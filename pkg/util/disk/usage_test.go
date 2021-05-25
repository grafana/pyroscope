package disk

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var _ = Describe("disk package", func() {
	testing.WithConfig(func(cfg **config.Config) {
		Describe("FreeSpace", func() {
			It("doesn't return an error", func() {
				_, err := FreeSpace((*cfg).Server.StoragePath)
				Expect(err).To(Not(HaveOccurred()))
			})

			It("returns non-zero value for storage space", func() {
				Expect(FreeSpace((*cfg).Server.StoragePath)).To(BeNumerically(">", 0))
			})
		})
	})
})
