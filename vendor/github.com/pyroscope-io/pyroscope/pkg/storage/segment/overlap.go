package segment

import (
	"math/big"
	"time"
)

func tmin(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func tmax(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func dmax(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

//  relationship                               overlap read             overlap write
// 	inside  rel = iota   // | S E |            <1                       1/1
// 	match                // matching ranges    1/1                      1/1
// 	outside              // | | S E            0/1                      0/1
// 	overlap              // | S | E            <1                       <1
// 	contain              // S | | E            1/1                      <1

// t1, t2 represent segment node, st, et represent the read query time range
func overlapRead(t1, t2, st, et time.Time, dur time.Duration) *big.Rat {
	m := int64(dmax(0, tmin(t2, et).Sub(tmax(t1, st))) / dur)
	d := int64(t2.Sub(t1) / dur)
	return big.NewRat(m, d)
}

// t1, t2 represent segment node, st, et represent the write query time range
func overlapWrite(t1, t2, st, et time.Time, dur time.Duration) *big.Rat {
	m := int64(dmax(0, tmin(t2, et).Sub(tmax(t1, st))) / dur)
	d := int64(et.Sub(st) / dur)
	return big.NewRat(m, d)
}
