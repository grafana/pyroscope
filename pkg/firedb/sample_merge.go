package firedb

import (
	"context"
	"fmt"
	"sort"

	"github.com/samber/lo"
	"github.com/segmentio/parquet-go"

	query "github.com/grafana/fire/pkg/firedb/query"
	"github.com/grafana/fire/pkg/iter"
)

func mergeSamplesByStacktraces(file *parquet.File, rows iter.Iterator[*Profile]) (iter.Iterator[StacktraceValue], error) {
	stacktraceIDCol, _ := query.GetColumnIndexByPath(file, "Samples.list.element.StacktraceID")
	if stacktraceIDCol == -1 {
		return nil, fmt.Errorf("no stacktrace id column found")
	}
	valuesCol, _ := query.GetColumnIndexByPath(file, "Samples.list.element.Values.list.element")
	if valuesCol == -1 {
		return nil, fmt.Errorf("no values column found")
	}
	it := query.NewJoinIterator(
		0,
		[]query.Iterator{
			query.NewRowNumberIterator(rows),
			query.NewColumnIterator(context.Background(), file.RowGroups(), stacktraceIDCol, "Samples.list.element.StacktraceID", 1024, nil, "StacktraceID"),
			query.NewColumnIterator(context.Background(), file.RowGroups(), valuesCol, "Samples.list.element.Values.list.element", 1024, nil, "Value"),
		}, nil,
	)
	var series [][]parquet.Value
	stacktraceAggrValues := map[int64]int64{}
	for it.Next() {
		values := it.At()
		series = values.Columns(series, "StacktraceID", "Value")
		for i := 0; i < len(series[0]); i++ {
			stacktraceAggrValues[series[0][i].Int64()] += series[1][i].Int64()
		}
	}
	keys := lo.Keys(stacktraceAggrValues)
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return &StacktraceValueIterator{
		aggregation: stacktraceAggrValues,
		sortedIds:   keys,
	}, nil
}

func mergeSamplesByProfile(file *parquet.File, rows iter.Iterator[*Profile]) (iter.Iterator[SeriesValue], error) {
	valuesCol, _ := query.GetColumnIndexByPath(file, "Samples.list.element.Values.list.element")
	if valuesCol == -1 {
		return nil, fmt.Errorf("no values column found")
	}
	it := query.NewJoinIterator(
		0,
		[]query.Iterator{
			query.NewRowNumberIterator(rows),
			query.NewColumnIterator(context.Background(), file.RowGroups(), valuesCol, "Samples.list.element.Values.list.element", 1024, nil, "Value"),
		}, nil,
	)
	var series [][]parquet.Value
	stacktraceAggrValues := map[int64]int64{}
	for it.Next() {
		values := it.At()
		p := values.Entries[0].RowValue.(*Profile)
		fmt.Println(p)
		series = values.Columns(series, "Value")
		for i := 0; i < len(series[0]); i++ {
			stacktraceAggrValues[series[0][i].Int64()] += series[1][i].Int64()
		}
	}
	// todo
	return nil, nil
}

type StacktraceValueIterator struct {
	aggregation map[int64]int64
	sortedIds   []int64
	curr        StacktraceValue
}

func (p *StacktraceValueIterator) Next() bool {
	if len(p.sortedIds) == 0 {
		return false
	}
	p.curr = StacktraceValue{
		StacktraceID: p.sortedIds[0],
		Value:        p.aggregation[p.sortedIds[0]],
	}
	p.sortedIds = p.sortedIds[1:]
	return true
}

func (p *StacktraceValueIterator) At() StacktraceValue {
	return p.curr
}

func (p *StacktraceValueIterator) Err() error {
	return nil
}

func (p *StacktraceValueIterator) Close() error {
	return nil
}
