package cli

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var _ = Describe("flags", func() {
	Describe("generateRootCmd", func() {
		testing.WithConfig(func(cfg **config.Config) {
			It("returns rootCmd", func() {
				rootCmd := generateRootCmd(*cfg)
				Expect(rootCmd).ToNot(BeNil())
			})
		})
	})
})
