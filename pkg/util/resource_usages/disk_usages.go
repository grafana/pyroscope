package resource_usages

import (
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"syscall"
)

const (
	B  = 1
	KB = 1024 * B
	MB = 1024 * KB
	GB = 1024 * MB
)

type DiskUsagesStats struct {
	All            uint64
	Used           uint64
	Free           uint64
	UsedPercentage float64
	FreePercentage float64
}

func DiskUsage(path string) (*DiskUsagesStats, error) {
	fs := syscall.Statfs_t{}
	err := syscall.Statfs(path, &fs)
	if err != nil {
		return nil, err
	}

	stat := DiskUsagesStats{}
	stat.All = fs.Blocks * uint64(fs.Bsize)
	stat.Free = fs.Bfree * uint64(fs.Bsize)
	stat.Used = stat.All - stat.Free

	stat.UsedPercentage = (float64(stat.Used) / float64(stat.All)) * 100
	stat.FreePercentage = (float64(stat.Free) / float64(stat.All)) * 100
	return &stat, nil
}

func IsRunningOutOfSpace(cfg *config.Config) bool {
	stats, err := DiskUsage("/")
	if err != nil {
		return false
	}

	if stats.FreePercentage < cfg.Server.OutOfSpaceThreshold {
		if stats.Free < uint64(cfg.Server.OutOfSpaceStaticThreshold)*uint64(MB) {
			return true
		}
		return false
	}
	return false
}

func ShouldShowOutOfSpaceWarning(cfg *config.Config) bool {
	stats, err := DiskUsage("/")
	if err != nil {
		return false
	}

	if stats.FreePercentage < cfg.Server.OutOfSpaceWarningThreshold {
		if stats.Free < uint64(cfg.Server.OutOfSpaceWarningStaticThreshold)*uint64(MB) {
			return true
		}
		return false
	}
	return false
}
