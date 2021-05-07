package disk

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"testing"
)

var _ = Describe("testDiskUsage", func() {
	It("canGetDiskUsage", func() {
		cfg := config.New()
		_, err := usage(cfg.Server.StoragePath)
		Expect(err).To(nil)
	})

	It("isNotRunningOutOfSpace", func() {
		cfg := config.New()
		result := IsRunningOutOfSpace(cfg.Server.StoragePath, cfg.Server.OutOfSpaceThreshold)
		Expect(result).To(BeFalse())
	})

	It("isRunningOutOfSpace", func() {
		cfg := config.New()
		stats, err := usage(cfg.Server.StoragePath)
		Expect(err).To(nil)

		cfg.Server.OutOfSpaceThreshold = stats.All
		result := IsRunningOutOfSpace(cfg.Server.StoragePath, cfg.Server.OutOfSpaceThreshold)
		Expect(result).To(BeTrue())
	})

	It("shouldNotShowOutOfSpaceWarning", func() {
		cfg := config.New()
		result := ShouldShowOutOfSpaceWarning(cfg.Server.StoragePath, cfg.Server.OutOfSpaceWarningThreshold)
		Expect(result).To(BeFalse())
	})

	It("shouldShowOutOfSpaceWarning", func() {
		cfg := config.New()

		stats, err := usage(cfg.Server.StoragePath)
		Expect(err).To(nil)

		cfg.Server.OutOfSpaceWarningThreshold = stats.All

		result := ShouldShowOutOfSpaceWarning(cfg.Server.StoragePath, cfg.Server.OutOfSpaceWarningThreshold)
		Expect(result).To(BeTrue())
	})
})

func TestDiskUsage(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Disk usage suite")
}
