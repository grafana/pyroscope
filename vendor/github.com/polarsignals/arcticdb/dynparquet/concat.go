package dynparquet

import "github.com/segmentio/parquet-go"

type concatenatedDynamicRowGroup struct {
	parquet.RowGroup
	dynamicColumns map[string][]string
}

func Concat(drg ...DynamicRowGroup) DynamicRowGroup {
	rg := make([]parquet.RowGroup, 0, len(drg))
	for _, d := range drg {
		rg = append(rg, d)
	}

	return &concatenatedDynamicRowGroup{
		RowGroup:       parquet.MultiRowGroup(rg...),
		dynamicColumns: drg[0].DynamicColumns(),
	}
}

func (c *concatenatedDynamicRowGroup) DynamicColumns() map[string][]string {
	return c.dynamicColumns
}

func (c *concatenatedDynamicRowGroup) DynamicRows() DynamicRowReader {
	return newDynamicRowGroupReader(c)
}
