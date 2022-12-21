package v1

import (
	"github.com/segmentio/parquet-go"

	profilev1 "github.com/grafana/phlare/pkg/gen/google/v1"
)

var (
	mappingsSchema = parquet.SchemaOf(&profilev1.Mapping{})
)

type MappingPersister struct{}

func (*MappingPersister) Name() string {
	return "mappings"
}

func (*MappingPersister) Schema() *parquet.Schema {
	return mappingsSchema
}

func (*MappingPersister) SortingColumns() parquet.SortingOption {
	return parquet.SortingColumns(
		parquet.Ascending("Id"),
		parquet.Ascending("BuildId"),
		parquet.Ascending("FileName"),
	)
}

func (*MappingPersister) Deconstruct(row parquet.Row, id uint64, m *profilev1.Mapping) parquet.Row {
	row = mappingsSchema.Deconstruct(row, m)
	return row
}

func (*MappingPersister) Reconstruct(row parquet.Row) (id uint64, m *profilev1.Mapping, err error) {
	var mapping profilev1.Mapping
	if err := mappingsSchema.Reconstruct(&mapping, row); err != nil {
		return 0, nil, err
	}
	return 0, &mapping, nil
}
