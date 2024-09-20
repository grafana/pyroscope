package model

import (
	"reflect"
	"time"

	"github.com/prometheus/common/model"
)

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

func GetSafeTimeRange(now time.Time, req any) model.Interval {
	if r, ok := req.(TimeRangeRequest); ok {
		x, ok := GetTimeRange(r)
		if ok {
			return x
		}
	}
	return model.Interval{
		Start: model.Time(now.Add(-time.Hour).UnixMilli()),
		End:   model.Time(now.UnixMilli()),
	}
}

func SetTimeRange(r interface{}, startTime, endTime model.Time) bool {
	const startFieldName = "Start"
	const endFieldName = "End"
	defer func() { _ = recover() }()
	v := reflect.ValueOf(r).Elem()
	startField := v.FieldByName(startFieldName)
	endField := v.FieldByName(endFieldName)
	if !startField.IsValid() || !endField.IsValid() {
		return false
	}
	if startField.Kind() != reflect.Int64 || endField.Kind() != reflect.Int64 {
		return false
	}
	startField.SetInt(int64(startTime))
	endField.SetInt(int64(endTime))
	return true
}
