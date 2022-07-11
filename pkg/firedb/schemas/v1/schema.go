package v1

import (
	"github.com/segmentio/parquet-go"

	fireparquet "github.com/grafana/fire/pkg/parquet"
)

var (
	stringsSchema = parquet.NewSchema("String", fireparquet.Group{
		fireparquet.NewGroupField("ID", parquet.Encoded(parquet.Uint(64), &parquet.DeltaBinaryPacked)),
		fireparquet.NewGroupField("String", parquet.Encoded(parquet.String(), &parquet.RLEDictionary)),
	})
)

func StringsSchema() *parquet.Schema {
	return stringsSchema
}

func StringsSorting() interface {
	parquet.RowGroupOption
	parquet.WriterOption
} {
	return parquet.SortingColumns(
		parquet.Ascending("ID"),
		parquet.Ascending("String"),
	)
}

type Strings []string

type storedString struct {
	ID     uint64 `parquet:",delta"`
	String string `parquet:",dict"`
}

func (s Strings) ToRows() []parquet.Row {
	var (
		rows   = make([]parquet.Row, len(s))
		stored storedString
	)

	for pos := range s {
		stored.ID = uint64(pos)
		stored.String = s[pos]
		rows[pos] = stringsSchema.Deconstruct(rows[pos], &stored)
	}
	return rows
}

func StringsFromRows(rows []parquet.Row) (Strings, error) {
	var (
		s      = make(Strings, len(rows))
		stored storedString
	)

	for pos := range rows {
		stored.String = ""
		if err := stringsSchema.Reconstruct(&stored, rows[pos]); err != nil {
			return nil, err
		}
		s[stored.ID] = stored.String
	}

	return s, nil
}
