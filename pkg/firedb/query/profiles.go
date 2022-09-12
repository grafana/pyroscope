package query

// type ProfileValue struct {
// 	Profile
// 	Value int64
// }

// type SeriesIterator struct {
// 	rowNums Iterator

// 	curr   Profile
// 	buffer [][]parquet.Value
// }

// func NewSeriesIterator(rowNums Iterator, fp model.Fingerprint, lbs firemodel.Labels) *SeriesIterator {
// 	return &SeriesIterator{
// 		rowNums: rowNums,
// 		curr:    Profile{fp: fp, labels: lbs},
// 	}
// }

// func (p *SeriesIterator) Next() bool {
// 	if !p.rowNums.Next() {
// 		return false
// 	}
// 	if p.buffer == nil {
// 		p.buffer = make([][]parquet.Value, 2)
// 	}
// 	result := p.rowNums.At()
// 	p.curr.RowNum = result.RowNumber[0]
// 	p.buffer = result.Columns(p.buffer, "TimeNanos")
// 	p.curr.t = model.TimeFromUnixNano(p.buffer[0][0].Int64())
// 	return true
// }

// func (p *SeriesIterator) At() Profile {
// 	return p.curr
// }

// func (p *SeriesIterator) Err() error {
// 	return p.rowNums.Err()
// }

// func (p *SeriesIterator) Close() error {
// 	return p.rowNums.Close()
// }
