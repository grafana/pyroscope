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
	var tmpDir *testing.TmpDirectory
	var cfg *config.Config

	BeforeEach(func() {
		tmpDir = testing.TmpDirSync()
		cfg = config.NewForTests(tmpDir.Path)
	})

	AfterEach(func() {
		tmpDir.Close()
	})

	Describe("Cli", func() {
		Context("no arguments", func() {
			It("returns error", func() {
				err := Cli(cfg, []string{})
				Expect(err).To(MatchError("no arguments passed"))
			})
		})
		Context("simple case", func() {
			It("returns nil", func() {
				cfg.Exec.SpyName = "debugspy"
				err := Cli(cfg, []string{"ls"})
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
