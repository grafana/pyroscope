package v1

import (
	"github.com/segmentio/parquet-go"

	phlareparquet "github.com/grafana/phlare/pkg/parquet"
)

var stringsSchema = parquet.NewSchema("String", phlareparquet.Group{
	phlareparquet.NewGroupField("ID", parquet.Encoded(parquet.Uint(64), &parquet.DeltaBinaryPacked)),
	phlareparquet.NewGroupField("String", parquet.Encoded(parquet.String(), &parquet.RLEDictionary)),
})

type StringPersister struct{}

func (*StringPersister) Name() string { return "strings" }

func (*StringPersister) Schema() *parquet.Schema { return stringsSchema }

func (*StringPersister) SortingColumns() parquet.SortingOption { return parquet.SortingColumns() }

func (*StringPersister) Deconstruct(row parquet.Row, id uint64, s string) parquet.Row {
	if cap(row) < 2 {
		row = make(parquet.Row, 0, 2)
	}
	row = row[:0]
	row = append(row, parquet.Int64Value(int64(id)).Level(0, 0, 0))
	row = append(row, parquet.ByteArrayValue([]byte(s)).Level(0, 0, 1))
	return row
}

func (*StringPersister) Reconstruct(row parquet.Row) (id uint64, s string, err error) {
	return 0, row[1].String(), nil
}
