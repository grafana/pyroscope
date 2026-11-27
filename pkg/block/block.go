package block

import (
	"github.com/grafana/pyroscope/v2/pkg/tenant"
)

const (
	DirNameSegment    = "segments"
	DirNameBlock      = "blocks"
	DirNameDLQ        = "dlq"
	DirNameAnonTenant = tenant.DefaultTenantID

	FileNameProfilesParquet = "profiles.parquet"
	FileNameDataObject      = "block.bin"
	FileNameMetadataObject  = "meta.pb"
)

const (
	defaultObjectSizeLoadInMemory = 1 << 20

	// DefaultTenantDatasetSizeLoadInMemory is the exclusive size threshold
	// below which datasets are buffered when their object is not in memory.
	DefaultTenantDatasetSizeLoadInMemory = 1 << 20

	maxRowsPerRowGroup  = 10 << 10
	symbolsPrefetchSize = 32 << 10
	localReadBufferSize = 64 << 10
)

func estimateReadBufferSize(s int64) int {
	const minSize = 64 << 10
	const maxSize = 1 << 20
	// Parquet has global buffer map, where buffer size is key,
	// so we want a low cardinality here.
	e := nextPowerOfTwo(uint32(s / 10))
	if e < minSize {
		return minSize
	}
	return int(min(e, maxSize))
}

// This is a verbatim copy of estimateReadBufferSize.
// It's kept for the sake of clarity and to avoid confusion.
func estimatePageBufferSize(s int64) int {
	const minSize = 64 << 10
	const maxSize = 1 << 20
	e := nextPowerOfTwo(uint32(s / 10))
	if e < minSize {
		return minSize
	}
	return int(min(e, maxSize))
}

func estimateFooterSize(size int64) int64 {
	var s int64
	// Prefetch enough of the file tail to cover the Parquet footer as well as
	// its preceding ColumnIndex and OffsetIndex sections in a single range read
	// (empirically ~1/420th of the section size on multi-row-group blocks).
	if size > 0 {
		s = size / 256
	}
	// set a minimum footer size of 64KiB
	if s < 64<<10 {
		s = 64 << 10
	}
	// set a maximum footer size of 512KiB
	if s > 512<<10 {
		s = 512 << 10
	}
	// now check clamp it to the actual size of the whole object
	if s > size {
		s = size
	}
	return s
}

func nextPowerOfTwo(n uint32) uint32 {
	if n == 0 {
		return 1
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n++
	return n
}
