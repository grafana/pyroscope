package pyroscope

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseInputMetadata_PreservesOriginalStartTime(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		query string
		want  int64
	}{
		{name: "missing"},
		{name: "empty", query: "&from="},
		{name: "zero", query: "&from=0"},
		{name: "seconds", query: "&from=1700000000", want: 1700000000000000000},
		{name: "milliseconds", query: "&from=1700000000123", want: 1700000000123000000},
		{name: "nanoseconds", query: "&from=1700000000123456789", want: 1700000000123456789},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := ingestHandler{}
			req := httptest.NewRequest(http.MethodPost, "/ingest?name=test"+tt.query, nil)
			before := time.Now()
			input, err := h.parseInputMetadataFromRequest(t.Context(), req)
			require.NoError(t, err)
			assert.Equal(t, tt.want, input.Metadata.OriginalStartTimeNanos)
			if tt.query == "" || tt.query == "&from=" {
				assert.False(t, input.Metadata.StartTime.Before(before))
				assert.False(t, input.Metadata.StartTime.After(time.Now()))
			} else {
				assert.Equal(t, tt.want, input.Metadata.StartTime.UnixNano())
			}
		})
	}
}
