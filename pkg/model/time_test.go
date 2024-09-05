package model

import (
	"testing"

	"github.com/stretchr/testify/assert"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func Test_SetTimeRange(t *testing.T) {
	t.Run("Type has time range fields", func(t *testing.T) {
		r := new(typesv1.LabelNamesRequest)
		assert.True(t, SetTimeRange(r, 1, 2))
		assert.Equal(t, int64(1), r.Start)
		assert.Equal(t, int64(2), r.End)
	})
	t.Run("Type has no time range fields", func(t *testing.T) {
		r := new(struct{})
		assert.False(t, SetTimeRange(r, 1, 2))
	})
}
