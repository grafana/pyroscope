package pprof

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/api/model/labelset"
	"github.com/grafana/pyroscope/v2/pkg/og/ingestion"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

func TestParseToPprof_OriginalTimestampPrecedence(t *testing.T) {
	t.Parallel()
	const embeddedTime int64 = 1700000000000000000
	const metadataTime int64 = 1700000100000000000
	for _, tt := range []struct {
		name     string
		embedded int64
		original int64
		want     int64
	}{
		{name: "embedded only", embedded: embeddedTime, want: embeddedTime},
		{name: "embedded takes precedence", embedded: embeddedTime, original: metadataTime, want: embeddedTime},
		{name: "supplied metadata fallback", original: metadataTime, want: metadataTime},
		{name: "server default is not supplied"},
		{name: "preserve nonzero timestamp before fixTime", embedded: 1000, original: metadataTime, want: 1000},
		{name: "preserve negative timestamp", embedded: -1, original: metadataTime, want: -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			profile := &profilev1.Profile{
				TimeNanos:   tt.embedded,
				StringTable: []string{"", "cpu", "nanoseconds"},
				SampleType:  []*profilev1.ValueType{{Type: 1, Unit: 2}},
			}
			raw, err := profile.MarshalVT()
			require.NoError(t, err)
			md := ingestion.Metadata{
				StartTime:              time.Unix(0, metadataTime),
				OriginalStartTimeNanos: tt.original,
				LabelSet:               labelset.New(map[string]string{"service_name": "test"}),
			}
			result, err := (&RawProfile{RawData: raw}).ParseToPprof(tenant.InjectTenantID(t.Context(), "tenant"), md, validation.MockLimits{})
			require.NoError(t, err)
			require.Len(t, result.Series, 1)
			assert.Equal(t, tt.want, result.Series[0].OriginalTimeNanos)
			if tt.embedded == 0 || tt.embedded == 1000 {
				assert.Equal(t, metadataTime, result.Series[0].Profile.TimeNanos, "fixTime still supplies the timestamp used for storage")
			}
		})
	}
}
