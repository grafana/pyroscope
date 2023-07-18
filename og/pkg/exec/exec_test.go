//go:build debugspy
// +build debugspy

// ^ this test requires debugspy to be enabled so to run this test make sure to include -tags debugspy

package exec

import (
	. "github.com/onsi/ginkgo/v2"
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
					_, err := NewExec(&(*cfg).Exec, []string{})
					Expect(err).To(MatchError("no arguments passed"))
				})
			})
			Context("simple case", func() {
				It("returns nil", func() {
					(*cfg).Exec.SpyName = "debugspy"
					_, err := NewExec(&(*cfg).Exec, []string{"ls"})
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})
	})
})
