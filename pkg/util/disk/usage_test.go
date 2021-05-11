package disk

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var _ = Describe("disk package", func() {
	testing.WithConfig(func(cfg **config.Config) {
		Describe("usage", func() {
			It("doesn't return an error", func() {
				_, err := usage((*cfg).Server.StoragePath)
				Expect(err).To(Not(HaveOccurred()))
			})
		})

		Describe("isRunningOutOfSpace", func() {
			Context("when there's enough space", func() {
				It("returns false", func() {
					result := IsRunningOutOfSpace((*cfg).Server.StoragePath, (*cfg).Server.OutOfSpaceThreshold)
					Expect(result).To(BeFalse())
				})
			})

			Context("when there's not enough space", func() {
				It("returns true", func() {
					stats, err := usage((*cfg).Server.StoragePath)
					Expect(err).To(Not(HaveOccurred()))

					(*cfg).Server.OutOfSpaceThreshold = stats.All
					result := IsRunningOutOfSpace((*cfg).Server.StoragePath, (*cfg).Server.OutOfSpaceThreshold)
					Expect(result).To(BeTrue())
				})
			})
		})
	})
})
