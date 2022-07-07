package schema

import (
	"testing"

	"github.com/segmentio/parquet-go"
	"github.com/stretchr/testify/require"

	"github.com/grafana/fire/pkg/firedb"
	v1 "github.com/grafana/fire/pkg/firedb/schemas/v1"
)

func TestSchema(t *testing.T) {

	originalSchema := parquet.SchemaOf(&firedb.Profile{})

	v1Schema := v1.ProfilesSchema()
	require.Equal(t, originalSchema.String(), v1Schema.String())
}
