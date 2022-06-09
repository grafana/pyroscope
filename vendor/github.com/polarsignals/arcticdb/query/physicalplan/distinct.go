package physicalplan

import (
	"hash/maphash"

	"github.com/apache/arrow/go/v8/arrow"
	"github.com/apache/arrow/go/v8/arrow/array"
	"github.com/apache/arrow/go/v8/arrow/memory"
	"github.com/apache/arrow/go/v8/arrow/scalar"

	"github.com/polarsignals/arcticdb/query/logicalplan"
)

type Distinction struct {
	pool     memory.Allocator
	seen     map[uint64]struct{}
	next     func(r arrow.Record) error
	columns  []logicalplan.ColumnMatcher
	hashSeed maphash.Seed
}

func Distinct(pool memory.Allocator, columns []logicalplan.ColumnMatcher) *Distinction {
	return &Distinction{
		pool:     pool,
		columns:  columns,
		seen:     make(map[uint64]struct{}),
		hashSeed: maphash.MakeSeed(),
	}
}

func (d *Distinction) SetNextCallback(callback func(r arrow.Record) error) {
	d.next = callback
}

func (d *Distinction) Callback(r arrow.Record) error {
	distinctFields := make([]arrow.Field, 0, 10)
	distinctFieldHashes := make([]uint64, 0, 10)
	distinctArrays := make([]arrow.Array, 0, 10)

	for i, field := range r.Schema().Fields() {
		for _, col := range d.columns {
			if col.Match(field.Name) {
				distinctFields = append(distinctFields, field)
				distinctFieldHashes = append(distinctFieldHashes, scalar.Hash(d.hashSeed, scalar.NewStringScalar(field.Name)))
				distinctArrays = append(distinctArrays, r.Column(i))
			}
		}
	}

	resBuilders := make([]array.Builder, 0, len(distinctArrays))
	for _, arr := range distinctArrays {
		resBuilders = append(resBuilders, array.NewBuilder(d.pool, arr.DataType()))
	}
	rows := int64(0)

	numRows := int(r.NumRows())

	colHashes := make([][]uint64, len(distinctFields))
	for i, arr := range distinctArrays {
		colHashes[i] = hashArray(arr)
	}

	for i := 0; i < numRows; i++ {
		hash := uint64(0)
		for j := range colHashes {
			if colHashes[j][i] == 0 {
				continue
			}

			hash = hashCombine(
				hash,
				hashCombine(
					distinctFieldHashes[j],
					colHashes[j][i],
				),
			)
		}

		if _, ok := d.seen[hash]; ok {
			continue
		}

		for j, arr := range distinctArrays {
			err := appendValue(resBuilders[j], arr, i)
			if err != nil {
				return err
			}
		}

		rows++
		d.seen[hash] = struct{}{}
	}

	if rows == 0 {
		// No need to call anything further down the chain, no new values were
		// seen so we can skip.
		return nil
	}

	resArrays := make([]arrow.Array, 0, len(resBuilders))
	for _, builder := range resBuilders {
		resArrays = append(resArrays, builder.NewArray())
	}

	schema := arrow.NewSchema(distinctFields, nil)

	distinctRecord := array.NewRecord(
		schema,
		resArrays,
		rows,
	)

	err := d.next(distinctRecord)
	distinctRecord.Release()
	return err
}
