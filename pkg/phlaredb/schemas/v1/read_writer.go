package v1

import (
	"github.com/parquet-go/parquet-go"
)

type PersisterName interface {
	Name() string
}

type Persister[T any] interface {
	PersisterName
	Schema() *parquet.Schema
	Deconstruct(parquet.Row, T) parquet.Row
	Reconstruct(parquet.Row) (T, error)
}
