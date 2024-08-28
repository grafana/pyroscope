package v1

import (
	"github.com/parquet-go/parquet-go"

	phlareparquet "github.com/grafana/pyroscope/pkg/parquet"
)

var stacktracesSchema = parquet.NewSchema("Stacktrace", phlareparquet.Group{
	phlareparquet.NewGroupField("ID", parquet.Encoded(parquet.Uint(64), &parquet.DeltaBinaryPacked)),
	phlareparquet.NewGroupField("LocationIDs", parquet.List(parquet.Encoded(parquet.Uint(64), &parquet.DeltaBinaryPacked))),
})

type Stacktrace struct {
	LocationIDs []uint64 `parquet:",list"`
}

type storedStacktrace struct {
	ID          uint64   `parquet:",delta"`
	LocationIDs []uint64 `parquet:",list"`
}

type StacktracePersister struct{}

func (*StacktracePersister) Name() string {
	return "stacktraces"
}

func (*StacktracePersister) Schema() *parquet.Schema {
	return stacktracesSchema
}

func (*StacktracePersister) Deconstruct(row parquet.Row, s *Stacktrace) parquet.Row {
	var stored storedStacktrace
	stored.LocationIDs = s.LocationIDs
	row = stacktracesSchema.Deconstruct(row, &stored)
	return row
}

func (*StacktracePersister) Reconstruct(row parquet.Row) (s *Stacktrace, err error) {
	var stored storedStacktrace
	if err := stacktracesSchema.Reconstruct(&stored, row); err != nil {
		return nil, err
	}
	return &Stacktrace{LocationIDs: stored.LocationIDs}, nil
}
