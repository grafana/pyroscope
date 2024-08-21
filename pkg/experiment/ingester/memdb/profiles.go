package memdb

import (
	"bytes"
	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/util/build"
	"github.com/parquet-go/parquet-go"
	"github.com/pkg/errors"
)

const (
	SegmentsParquetWriteBufferSize = 8 * 0x1000
)

func WriteProfiles(metrics *HeadMetrics, profiles []v1.InMemoryProfile) ([]byte, error) {
	buf := &bytes.Buffer{}
	w := parquet.NewGenericWriter[*v1.Profile](
		buf,
		parquet.PageBufferSize(SegmentsParquetWriteBufferSize),
		parquet.CreatedBy("github.com/grafana/pyroscope/", build.Version, build.Revision),
		v1.ProfilesSchema,
	)
	_, err := parquet.CopyRows(w, v1.NewInMemoryProfilesRowReader(profiles))
	if err != nil {
		return nil, errors.Wrap(err, "write row group segments to disk")
	}

	if err := w.Close(); err != nil {
		return nil, errors.Wrap(err, "close row group segment writer")
	}

	metrics.writtenProfileSegments.WithLabelValues("success").Inc()
	res := buf.Bytes()
	metrics.writtenProfileSegmentsBytes.Observe(float64(len(res)))
	metrics.rowsWritten.WithLabelValues("profiles").Add(float64(len(profiles)))
	return res, nil
}
