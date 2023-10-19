package model

import "github.com/prometheus/common/model"

// TimeRangeRequest is a request that has a time interval.
type TimeRangeRequest interface {
	GetStart() int64
	GetEnd() int64
}

// GetTimeRange returns the time interval and true if the request has an
// interval, otherwise ok is false.
func GetTimeRange(req TimeRangeRequest) (model.Interval, bool) {
	if req.GetStart() == 0 || req.GetEnd() == 0 {
		return model.Interval{}, false
	}
	return model.Interval{
		Start: model.Time(req.GetStart()),
		End:   model.Time(req.GetEnd()),
	}, true
}
