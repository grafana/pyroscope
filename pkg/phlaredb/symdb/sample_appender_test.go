package symdb

import (
	"testing"

	"github.com/stretchr/testify/assert"

	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

func Test_SampleAppender(t *testing.T) {
	for _, test := range []struct {
		description string
		assert      func(*testing.T, *SampleAppender)
	}{
		{
			description: "empty appender",
			assert: func(t *testing.T, a *SampleAppender) {
				assert.Equal(t, 0, a.Len())
			},
		},
		{
			description: "hashtable",
			assert: func(t *testing.T, a *SampleAppender) {
				a.Append(1, 1)
				a.AppendMany([]uint32{1337, 42}, []uint64{1, 1})
				a.Append(1337, 1)
				assert.Equal(t, 3, a.Len())
				assert.Equal(t, 3, len(a.hashmap))
				assert.Equal(t, schemav1.Samples{
					StacktraceIDs: []uint32{1, 42, 1337},
					Values:        []uint64{1, 1, 2},
				}, a.Samples())
			},
		},
		{
			description: "sparse",
			assert: func(t *testing.T, a *SampleAppender) {
				a.Append(4, 1)
				a.Append(1, 1)
				a.AppendMany([]uint32{20, 40}, []uint64{1, 1})
				a.Append(10, 1)
				a.Append(40, 1)
				assert.Equal(t, 5, a.Len())
				assert.Equal(t, 0, len(a.hashmap))
				assert.Equal(t, 11, len(a.chunks))
				assert.Equal(t, schemav1.Samples{
					StacktraceIDs: []uint32{1, 4, 10, 20, 40},
					Values:        []uint64{1, 1, 1, 1, 2},
				}, a.Samples())
			},
		},
	} {
		a := NewSampleAppenderSize(4, 4)
		test.assert(t, a)
	}
}
