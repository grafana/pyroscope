package disk

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/grafana/pyroscope/pkg/og/config"
	"github.com/grafana/pyroscope/pkg/og/testing"
)

var _ = Describe("disk package", func() {
	var (
		u   UsageStats
		err error
	)
	testing.WithConfig(func(cfg **config.Config) {
		BeforeEach(func() {
			u, err = Usage((*cfg).Server.StoragePath)
		})
		Describe("Usage", func() {
			It("doesn't return an error", func() {
				Expect(err).To(Not(HaveOccurred()))
			})

			It("returns non-zero Total", func() {
				Expect(u.Total).To(BeNumerically(">", 0))
			})

			It("returns non-zero Available", func() {
				Expect(u.Available).To(BeNumerically(">", 0))
			})

			It("returns Available < Total", func() {
				Expect(u.Available).To(BeNumerically("<", u.Total))
			})
		})
	})
})
