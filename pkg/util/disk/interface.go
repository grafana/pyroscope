package diskutil

type VolumeChecker interface {
	HasHighDiskUtilization(path string) (*VolumeStats, error)
}

type VolumeStats struct {
	BytesAvailable uint64
	BytesTotal     uint64

	HighDiskUtilization bool
}
