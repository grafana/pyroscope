package v1

import (
	"github.com/segmentio/parquet-go"

	fireparquet "github.com/grafana/fire/pkg/parquet"
)

var (
	stacktracesSchema = parquet.NewSchema("Stacktrace", fireparquet.Group{
		fireparquet.NewGroupField("ID", parquet.Encoded(parquet.Uint(64), &parquet.DeltaBinaryPacked)),
		fireparquet.NewGroupField("LocationIDs", parquet.Repeated(parquet.Uint(64))),
	})
)

type Stacktrace struct {
	LocationIDs []uint64 `parquet:","`
}

type storedStacktrace struct {
	ID          uint64   `parquet:",delta"`
	LocationIDs []uint64 `parquet:","`
}

type StacktracePersister struct {
}

func (*StacktracePersister) Schema() *parquet.Schema {
	return stacktracesSchema
}

func (*StacktracePersister) SortingColumns() SortingColumns {
	return parquet.SortingColumns(
		parquet.Ascending("ID"),
		parquet.Ascending("LocationIDs"),
	)
}

func (*StacktracePersister) Deconstruct(row parquet.Row, id uint64, s *Stacktrace) parquet.Row {
	var stored storedStacktrace
	stored.ID = uint64(id)
	stored.LocationIDs = s.LocationIDs
	row = stacktracesSchema.Deconstruct(row, &stored)
	return row
}

func (*StacktracePersister) Reconstruct(row parquet.Row) (id uint64, s *Stacktrace, err error) {
	var stored storedStacktrace
	if err := stacktracesSchema.Reconstruct(&stored, row); err != nil {
		return 0, nil, err
	}
	return stored.ID, &Stacktrace{LocationIDs: stored.LocationIDs}, nil
}
