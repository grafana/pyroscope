package v1

import (
	"github.com/segmentio/parquet-go"

	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
)

var functionsSchema = parquet.SchemaOf(&profilev1.Function{})

type FunctionPersister struct{}

func (*FunctionPersister) Name() string {
	return "functions"
}

func (*FunctionPersister) Schema() *parquet.Schema {
	return functionsSchema
}

func (*FunctionPersister) SortingColumns() SortingColumns {
	return parquet.SortingColumns(
		parquet.Ascending("Id"),
		parquet.Ascending("Name"),
		parquet.Ascending("FileName"),
	)
}

func (*FunctionPersister) Deconstruct(row parquet.Row, id uint64, l *profilev1.Function) parquet.Row {
	row = functionsSchema.Deconstruct(row, l)
	return row
}

func (*FunctionPersister) Reconstruct(row parquet.Row) (id uint64, l *profilev1.Function, err error) {
	var function profilev1.Function
	if err := functionsSchema.Reconstruct(&function, row); err != nil {
		return 0, nil, err
	}
	return 0, &function, nil
}
