package v1

import (
	"testing"

	"github.com/segmentio/parquet-go"
	"github.com/stretchr/testify/require"
)

func TestSchemaMatch(t *testing.T) {
	originalSchema := parquet.SchemaOf(&Profile{})

	v1Schema := ProfilesSchema()
	require.Equal(t, originalSchema.String(), v1Schema.String())
}
