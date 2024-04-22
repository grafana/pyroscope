//go:build !windows

package diskutil

import (
	"fmt"
	"math"

	"golang.org/x/sys/unix"
)

type volumeChecker struct {
	minFreeDisk                uint64
	minDiskAvailablePercentage float64
}

func NewVolumeChecker(minFreeDisk uint64, minDiskAvailablePercentage float64) VolumeChecker {
	return &volumeChecker{
		minFreeDisk:                minFreeDisk,
		minDiskAvailablePercentage: minDiskAvailablePercentage,
	}
}

func (v *volumeChecker) HasHighDiskUtilization(path string) (*VolumeStats, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return nil, err
	}

	//nolint:unconvert // In BSD family (except OpenBSD), the type of stat.Bavail is int64.
	avail := uint64(stat.Bavail)
	isValid := avail&math.MaxInt64 == avail && stat.Blocks > 0
	if !isValid {
		return nil, fmt.Errorf("invalid statfs values: %+v", stat)
	}

	// available means accessible to the current user, while free means bytes
	// for privileged users. (Linux sometimes reserves some space for root)
	var (
		stats = VolumeStats{
			BytesAvailable: avail * uint64(stat.Bsize),
		}
		percentageAvailable = float64(avail) / float64(stat.Blocks*uint64(stat.Bsize))
	)

	// if bytes available is bigger than minFreeDisk => not in high disk utilization
	if stats.BytesAvailable >= v.minFreeDisk {
		return &stats, nil
	}

	// no in high disk utilization when more than the constant
	if percentageAvailable > v.minDiskAvailablePercentage {
		return &stats, nil
	}

	stats.HighDiskUtilization = true
	return &stats, nil
}
