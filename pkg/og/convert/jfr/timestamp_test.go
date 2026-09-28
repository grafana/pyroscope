package jfr

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/og/convert/pprof/bench"
)

func TestParseToPprof_PreservesOriginalTimestamp(t *testing.T) {
	raw, err := bench.ReadGzipFile("testdata/cortex-dev-01__kafka-0__cpu__0.jfr.gz")
	require.NoError(t, err)
	for _, format := range []string{"plain", "multipart"} {
		for _, tt := range []struct {
			name      string
			startTime int64
			original  int64
		}{
			{name: "supplied", startTime: 1700000000000000000, original: 1700000000000000000},
			{name: "default", startTime: 1700000000000000000},
			{name: "different default", startTime: 1700000100000000000},
			{name: "zero"},
		} {
			t.Run(format+"/"+tt.name, func(t *testing.T) {
				md := testMetadata()
				md.StartTime = time.Unix(0, tt.startTime)
				md.EndTime = md.StartTime.Add(time.Second)
				md.OriginalStartTimeNanos = tt.original
				p := &RawProfile{RawData: raw}
				if format == "multipart" {
					p.RawData, p.FormDataContentType = multipartJFR(t, gzipBytes(t, raw), nil)
				}
				result, err := p.ParseToPprof(testContext(), md, fixedMaxProfileSize(32<<20))
				require.NoError(t, err)
				require.NotEmpty(t, result.Series)
				for _, s := range result.Series {
					assert.Equal(t, tt.original, s.OriginalTimeNanos)
					assert.Equal(t, tt.startTime, s.Profile.TimeNanos, "the timestamp used for storage is unchanged")
				}
			})
		}
	}
}
