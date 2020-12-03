package bytesize

import "fmt"

type ByteSize int64

var Byte ByteSize = 1

var (
	KB = 1024 * Byte
	MB = 1024 * KB
	GB = 1024 * MB
	TB = 1024 * GB
	PB = 1024 * TB
)

var (
	KiB = 1000 * Byte
	MiB = 1000 * KiB
	GiB = 1000 * MiB
	TiB = 1000 * GiB
	PiB = 1000 * TiB
)

var suffixes = []string{"KB", "MB", "GB", "TB", "PB"}

func (b ByteSize) String() string {
	if b < KB {
		return fmt.Sprintf("%d bytes", b)
	}
	bf := float64(b)
	for _, s := range suffixes {
		bf /= 1024.0
		if bf < 1024 {
			return fmt.Sprintf("%.2f %s", bf, s)
		}
	}
	return fmt.Sprintf("%.2f %s", bf, suffixes[len(suffixes)-1])
}
