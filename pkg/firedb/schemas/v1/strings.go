package v1

import (
	"github.com/segmentio/parquet-go"

	fireparquet "github.com/grafana/fire/pkg/parquet"
)

var stringsSchema = parquet.NewSchema("String", fireparquet.Group{
	fireparquet.NewGroupField("ID", parquet.Encoded(parquet.Uint(64), &parquet.DeltaBinaryPacked)),
	fireparquet.NewGroupField("String", parquet.Encoded(parquet.String(), &parquet.RLEDictionary)),
})

type StoredString struct {
	ID     uint64 `parquet:",delta"`
	String string `parquet:",dict"`
}

type StringPersister struct{}

func (*StringPersister) Name() string {
	return "strings"
}

func (*StringPersister) Schema() *parquet.Schema {
	return stringsSchema
}

func (*StringPersister) SortingColumns() SortingColumns {
	return parquet.SortingColumns(
		parquet.Ascending("ID"),
		parquet.Ascending("String"),
	)
}

func (*StringPersister) Deconstruct(row parquet.Row, id uint64, s string) parquet.Row {
	var stored StoredString
	stored.ID = id
	stored.String = s
	row = stringsSchema.Deconstruct(row, &stored)
	return row
}

func (*StringPersister) Reconstruct(row parquet.Row) (id uint64, s string, err error) {
	var stored StoredString
	if err := stringsSchema.Reconstruct(&stored, row); err != nil {
		return 0, "", err
	}
	return stored.ID, stored.String, nil
}
