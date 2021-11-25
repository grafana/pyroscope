// +build debugspy
// ^ this test requires debugspy to be enabled so to run this test make sure to include -tags debugspy

package exec

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var _ = Describe("Cli", func() {
	disableMacOSChecks = true
	disableLinuxChecks = true

	testing.WithConfig(func(cfg **config.Config) {
		Describe("Exec", func() {
			Context("no arguments", func() {
				It("returns error", func() {
					_, err := NewConnect(&(*cfg).Connect, []string{})
					Expect(err).To(MatchError(UnsupportedSpyError{Subcommand: "connect", Args: []string{}}))
				})
			})
			Context("simple case", func() {
				It("returns nil", func() {
					(*cfg).Connect.SpyName = "debugspy"
					_, err := NewConnect(&(*cfg).Connect, []string{})
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})
	})
})
