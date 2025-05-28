package block

import (
	"github.com/grafana/pyroscope/pkg/tenant"
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
	defaultObjectSizeLoadInMemory        = 1 << 20
	defaultTenantDatasetSizeLoadInMemory = 1 << 20

	maxRowsPerRowGroup  = 10 << 10
	symbolsPrefetchSize = 32 << 10
)

const (
	// Each 2MB translates to an I/O read op.
	parquetReadBufferSize      = 2 << 20
	parquetPageWriteBufferSize = 1 << 20
)

func estimateFooterSize(size int64) int64 {
	var s int64
	// as long as we don't keep the exact footer sizes in the meta estimate it
	if size > 0 {
		s = size / 10000
	}
	// set a minimum footer size of 32KiB
	if s < 32<<10 {
		s = 32 << 10
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
