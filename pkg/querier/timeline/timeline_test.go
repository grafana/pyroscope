package timeline_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	"github.com/grafana/phlare/pkg/querier/timeline"
)

const TimelineStepSec = 10

func Test_No_Backfill(t *testing.T) {
	TestDate := time.Now()
	points := &typesv1.Series{
		Points: []*typesv1.Point{
			{Timestamp: TestDate.UnixMilli(), Value: 99},
		},
	}

	timeline := timeline.New(points, TestDate.UnixMilli(), TestDate.UnixMilli(), TimelineStepSec)

	assert.Equal(t, TestDate.UnixMilli()/1000, timeline.StartTime)
	assert.Equal(t, []uint64{
		99,
	}, timeline.Samples)
}

func Test_Backfill_Data_Start_End(t *testing.T) {
	TestDate := time.Now()
	startTime := TestDate.Add(-1 * time.Minute).UnixMilli()
	endTime := TestDate.Add(1 * time.Minute).UnixMilli()

	points := &typesv1.Series{
		Points: []*typesv1.Point{
			{Timestamp: TestDate.UnixMilli(), Value: 99},
		},
	}

	timeline := timeline.New(points, startTime, endTime, TimelineStepSec)

	assert.Equal(t, startTime/1000, timeline.StartTime)
	assert.Equal(t, []uint64{
		0, 0, 0, 0, 0,
		99,
		0, 0, 0, 0, 0,
	}, timeline.Samples)
}

func Test_Backfill_Data_Middle(t *testing.T) {
	TestDate := time.Now()
	startTime := TestDate.Add(-1 * time.Minute).UnixMilli()
	endTime := TestDate.Add(1 * time.Minute).UnixMilli()

	points := &typesv1.Series{
		Points: []*typesv1.Point{
			{Timestamp: TestDate.UnixMilli(), Value: 99},
			{Timestamp: TestDate.Add(20 * time.Second).UnixMilli(), Value: 98},
		},
	}

	timeline := timeline.New(points, startTime, endTime, TimelineStepSec)

	assert.Equal(t, startTime/1000, timeline.StartTime)
	assert.Equal(t, []uint64{
		0, 0, 0, 0, 0,
		99,
		0,
		98,
		0, 0, 0, 0,
	}, timeline.Samples)
}

func Test_Backfill_All(t *testing.T) {
	TestDate := time.Now()
	startTime := TestDate.Add(-1 * time.Minute).UnixMilli()
	endTime := TestDate.Add(1 * time.Minute).UnixMilli()

	points := &typesv1.Series{
		Points: []*typesv1.Point{},
	}

	timeline := timeline.New(points, startTime, endTime, TimelineStepSec)

	assert.Equal(t, startTime/1000, timeline.StartTime)
	assert.Equal(t, []uint64{
		0, 0, 0, 0, 0,
		0, 0, 0, 0, 0,
		0,
	}, timeline.Samples)
}
