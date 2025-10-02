package memdb

const segmentsParquetWriteBufferSize = 32 << 10

func WriteProfiles(metrics *HeadMetrics, profiles []InMemoryArrowProfile) ([]byte, error) {
	// Use the Arrow-based implementation for better performance
	return WriteProfilesArrow(metrics, profiles)
}
