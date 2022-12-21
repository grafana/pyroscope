package v1

import (
	"github.com/segmentio/parquet-go"

	profilev1 "github.com/grafana/phlare/pkg/gen/google/v1"
)

var (
	locationsSchema = parquet.SchemaOf(&profilev1.Location{})
)

type LocationPersister struct{}

func (*LocationPersister) Name() string {
	return "locations"
}
func (*LocationPersister) Schema() *parquet.Schema {
	return locationsSchema
}

func (*LocationPersister) SortingColumns() parquet.SortingOption {
	return parquet.SortingColumns(
		parquet.Ascending("Id"),
		parquet.Ascending("MappingId"),
		parquet.Ascending("Address"),
	)
}

func (*LocationPersister) Deconstruct(row parquet.Row, id uint64, l *profilev1.Location) parquet.Row {
	row = locationsSchema.Deconstruct(row, l)
	return row
}

func (*LocationPersister) Reconstruct(row parquet.Row) (id uint64, l *profilev1.Location, err error) {
	var location profilev1.Location
	if err := locationsSchema.Reconstruct(&location, row); err != nil {
		return 0, nil, err
	}
	return 0, &location, nil
}
