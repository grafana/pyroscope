package timeline_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/querier/timeline"
)

const timelineStepSec = 10

func Test_No_Backfill(t *testing.T) {
	TestDate := time.Now()
	points := &typesv1.Series{
		Points: []*typesv1.Point{
			{Timestamp: TestDate.UnixMilli(), Value: 99},
		},
	}

	timeline := timeline.New(points, TestDate.UnixMilli(), TestDate.UnixMilli(), timelineStepSec)

	assert.Equal(t, TestDate.UnixMilli()/1000, timeline.StartTime)
	assert.Equal(t, []uint64{99}, timeline.Samples)
}

func Test_Backfill_Data_Start_End(t *testing.T) {
	TestDate := time.Now()
	startTime := TestDate.Add(-1 * time.Minute).UnixMilli()
	endTime := TestDate.Add(1 * time.Minute).UnixMilli()

	points := &typesv1.Series{
		Points: []*typesv1.Point{
			//      0 ms    -60000 ms
			//  10000 ms    -50000 ms
			//  20000 ms    -40000 ms
			//  30000 ms    -30000 ms
			//  40000 ms    -20000 ms
			//  50000 ms    -10000 ms
			{Timestamp: TestDate.UnixMilli(), Value: 99},
			//  70000 ms    +10000 ms
			//  80000 ms    +20000 ms
			//  90000 ms    +30000 ms
			// 100000 ms    +40000 ms
			// 110000 ms    +50000 ms

		},
	}

	timeline := timeline.New(points, startTime, endTime, timelineStepSec)

	assert.Equal(t, startTime/1000, timeline.StartTime)
	assert.Equal(t, []uint64{
		0,  //      0 ms    -60000 ms
		0,  //  10000 ms    -50000 ms
		0,  //  20000 ms    -40000 ms
		0,  //  30000 ms    -30000 ms
		0,  //  40000 ms    -20000 ms
		0,  //  50000 ms    -10000 ms
		99, //  60000 ms         0 ms (now)
		0,  //  70000 ms    +10000 ms
		0,  //  80000 ms    +20000 ms
		0,  //  90000 ms    +30000 ms
		0,  // 100000 ms    +40000 ms
		0,  // 110000 ms    +50000 ms
	}, timeline.Samples)
}

func Test_Backfill_Data_Middle(t *testing.T) {
	TestDate := time.Now()
	startTime := TestDate.Add(-1 * time.Minute).UnixMilli()
	endTime := TestDate.Add(1 * time.Minute).UnixMilli()

	points := &typesv1.Series{
		Points: []*typesv1.Point{
			//      0 ms    -60000 ms
			//  10000 ms    -50000 ms
			//  20000 ms    -40000 ms
			//  30000 ms    -30000 ms
			//  40000 ms    -20000 ms
			//  50000 ms    -10000 ms
			{Timestamp: TestDate.UnixMilli(), Value: 99},
			//  70000 ms    +10000 ms
			{Timestamp: TestDate.Add(20 * time.Second).UnixMilli(), Value: 98},
			//  90000 ms    +30000 ms
			// 100000 ms    +40000 ms
			// 110000 ms    +50000 ms
		},
	}

	timeline := timeline.New(points, startTime, endTime, timelineStepSec)

	assert.Equal(t, startTime/1000, timeline.StartTime)
	assert.Equal(t, []uint64{
		0,  //      0 ms    -60000 ms
		0,  //  10000 ms    -50000 ms
		0,  //  20000 ms    -40000 ms
		0,  //  30000 ms    -30000 ms
		0,  //  40000 ms    -20000 ms
		0,  //  50000 ms    -10000 ms
		99, //  60000 ms         0 ms (now)
		0,  //  70000 ms    +10000 ms
		98, //  80000 ms    +20000 ms
		0,  //  90000 ms    +30000 ms
		0,  // 100000 ms    +40000 ms
		0,  // 110000 ms    +50000 ms
	}, timeline.Samples)
}

func Test_Backfill_All(t *testing.T) {
	TestDate := time.Now()
	startTime := TestDate.Add(-1 * time.Minute).UnixMilli()
	endTime := TestDate.Add(1 * time.Minute).UnixMilli()

	points := &typesv1.Series{
		Points: []*typesv1.Point{},
	}

	timeline := timeline.New(points, startTime, endTime, timelineStepSec)

	assert.Equal(t, startTime/1000, timeline.StartTime)
	assert.Equal(t, []uint64{
		0, //      0 ms    -60000 ms
		0, //  10000 ms    -50000 ms
		0, //  20000 ms    -40000 ms
		0, //  30000 ms    -30000 ms
		0, //  40000 ms    -20000 ms
		0, //  50000 ms    -10000 ms
		0, //  60000 ms         0 ms (now)
		0, //  70000 ms    +10000 ms
		0, //  80000 ms    +20000 ms
		0, //  90000 ms    +30000 ms
		0, // 100000 ms    +40000 ms
		0, // 110000 ms    +50000 ms
	}, timeline.Samples)
}

func Test_Backfill_Arbitrary(t *testing.T) {
	startMs := int64(0)
	endMs := int64(10 * time.Second / time.Millisecond)
	step := int64(1)
	series := &typesv1.Series{
		Points: []*typesv1.Point{
			//    0 ms
			// 1000 ms
			{Timestamp: 2000, Value: 69},
			{Timestamp: 3000, Value: 83},
			// 4000 ms
			// 5000 ms
			{Timestamp: 6000, Value: 85},
			// 7000 ms
			{Timestamp: 8000, Value: 91},
			// 9000 ms
		},
	}

	tl := timeline.New(series, startMs, endMs, step)
	assert.Equal(t, startMs/1000, tl.StartTime)

	assert.Equal(t, []uint64{
		0,  //    0 ms
		0,  // 1000 ms
		69, // 2000 ms
		83, // 3000 ms
		0,  // 4000 ms
		0,  // 5000 ms
		85, // 6000 ms
		0,  // 7000 ms
		91, // 8000 ms
		0,  // 9000 ms
	}, tl.Samples)
}
