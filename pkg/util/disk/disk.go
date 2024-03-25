//go:build !windows

package diskutil

import (
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

  // Convert [stat.Bavail] to uint64 safely, considering it can be negative, and represented
  // as an int64 on BSD systems.
  //
  // See https://cs.opensource.google/go/x/sys/+/refs/tags/v0.18.0:unix/ztypes_freebsd_arm64.go;l=96
  var bytesAvailable uint64
  if stat.Bavail < 0 {
    bytesAvailable = 0
  } else {
    bytesAvailable = uint64(stat.Bavail) * uint64(stat.Bsize)
  }

	// available means accessible to the current user, while free means bytes
	// for privileged users. (Linux sometimes reserves some space for root)
	var (
		stats = VolumeStats{
			BytesAvailable: bytesAvailable,
		}
		percentageAvailable = 0.0
	)

  // Ensure [stat.Blocks] is greater than zero to avoid division by zero.
  if stat.Blocks > 0 {  
    percentageAvailable = float64(bytesAvailable) / float64(uint64(stat.Blocks) * uint64(stat.Bsize))
  }

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
