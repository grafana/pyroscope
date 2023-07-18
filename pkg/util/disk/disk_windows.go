//go:build windows

package diskutil

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
	panic("not implemented")
}
