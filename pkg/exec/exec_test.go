//go:build debugspy
// +build debugspy

// ^ this test requires debugspy to be enabled so to run this test make sure to include -tags debugspy

package exec

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/testing"

	// revive:disable:blank-imports Depending on configuration these packages may or may not be used.
	//   That's why we do a blank import here and then packages themselves register with the rest of the code.

	_ "github.com/pyroscope-io/pyroscope/pkg/agent/debugspy"
	// revive:enable:blank-imports
)

var _ = Describe("Cli", func() {
	disableMacOSChecks = true
	disableLinuxChecks = true

	testing.WithConfig(func(cfg **config.Config) {
		Describe("Cli", func() {
			Context("no arguments", func() {
				It("returns error", func() {
					err := Cli(&(*cfg).Exec, []string{})
					Expect(err).To(MatchError("no arguments passed"))
				})
			})
			Context("simple case", func() {
				It("returns nil", func() {
					(*cfg).Exec.SpyName = "debugspy"
					err := Cli(&(*cfg).Exec, []string{"ls"})
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})
	})
})
