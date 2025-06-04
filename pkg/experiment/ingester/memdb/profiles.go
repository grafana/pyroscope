package memdb

import (
	"bytes"
	"fmt"

	"github.com/parquet-go/parquet-go"

	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/util/build"
)

const segmentsParquetWriteBufferSize = 32 << 10

func WriteProfiles(metrics *HeadMetrics, profiles []v1.InMemoryProfile) ([]byte, error) {
	buf := &bytes.Buffer{}
	w := parquet.NewGenericWriter[*v1.Profile](
		buf,
		parquet.PageBufferSize(segmentsParquetWriteBufferSize),
		parquet.CreatedBy("github.com/grafana/pyroscope/", build.Version, build.Revision),
		v1.ProfilesSchema,
	)

	if _, err := parquet.CopyRows(w, v1.NewInMemoryProfilesRowReader(profiles)); err != nil {
		return nil, fmt.Errorf("failed to write profile rows to parquet table: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to close parquet table: %w", err)
	}

	metrics.writtenProfileSegments.WithLabelValues("success").Inc()
	res := buf.Bytes()
	metrics.writtenProfileSegmentsBytes.Observe(float64(len(res)))
	metrics.rowsWritten.WithLabelValues("profiles").Add(float64(len(profiles)))
	return res, nil
}
