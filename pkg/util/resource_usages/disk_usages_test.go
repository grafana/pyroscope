package resource_usages

import (
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"testing"
)

func TestDiskUsage(t *testing.T) {
	cfg := config.New()
	stats, err := DiskUsage(cfg.Server.StoragePath)
	if err != nil {
		t.Error(err)
		return
	}

	t.Log("All: ", float64(stats.All)/float64(GB), " GB")
	t.Log("Free: ", float64(stats.Free)/float64(GB), " GB")
	t.Log("Used: ", float64(stats.Used)/float64(GB), " GB")
	t.Log("Free of %: ", stats.FreePercentage)
	t.Log("Used of %: ", stats.UsedPercentage)
}

func TestIsNotRunningOutOfSpace(t *testing.T) {
	if IsRunningOutOfSpace(config.New()) {
		t.Error("Running out of space")
		return
	}
}

func TestRunningOutOfSpace(t *testing.T) {
	cfg := config.New()
	cfg.Server.OutOfSpaceThreshold = 99
	cfg.Server.OutOfSpaceStaticThreshold = 300 * GB
	if !IsRunningOutOfSpace(cfg) {
		t.Error("should run out of space")
		return
	}
}

func TestShouldNotShowOutOfSpaceWarning(t *testing.T) {
	if ShouldShowOutOfSpaceWarning(config.New()) {
		t.Error("Should not show out of space warning")
		return
	}
}

func TestShouldShowOutOfSpaceWarning(t *testing.T) {
	cfg := config.New()
	cfg.Server.OutOfSpaceWarningThreshold = 99
	cfg.Server.OutOfSpaceWarningStaticThreshold = GB * 300

	if !ShouldShowOutOfSpaceWarning(cfg) {
		t.Error("Should show out of space warning")
		return
	}
}
