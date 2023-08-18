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
	const startMs = int64(1692395965)
	const endMs = startMs + (timelineStepSec * 1000)
	const stepSec = 10

	points := &typesv1.Series{
		Points: []*typesv1.Point{
			{Timestamp: startMs, Value: 99},
		},
	}

	timeline := timeline.New(points, startMs, endMs, stepSec)

	const snappedStartMs = int64(1692390000)
	assert.Equal(t, snappedStartMs/1000, timeline.StartTime)
	assert.Equal(t, []uint64{99}, timeline.Samples)
}

func Test_Backfill_Data_Start_End(t *testing.T) {
	const startMs = int64(1692397017190)
	const endMs = int64(1692397137190)
	const stepSec = 10

	points := &typesv1.Series{
		Points: []*typesv1.Point{
			// 1692397017190 ms
			// 1692397027190 ms
			// 1692397037190 ms
			// 1692397047190 ms
			// 1692397057190 ms
			// 1692397067190 ms
			{Timestamp: 1692397077190, Value: 99},
			// 1692397087190 ms
			// 1692397097190 ms
			// 1692397107190 ms
			// 1692397117190 ms
			// 1692397127190 ms
		},
	}

	timeline := timeline.New(points, startMs, endMs, stepSec)

	const snappedStartMs = int64(1692397010000)
	assert.Equal(t, snappedStartMs/1000, timeline.StartTime)
	assert.Equal(t, []uint64{
		0,  // 1692397010000 ms (1692397017190 ms)
		0,  // 1692397020000 ms (1692397027190 ms)
		0,  // 1692397030000 ms (1692397037190 ms)
		0,  // 1692397040000 ms (1692397047190 ms)
		0,  // 1692397050000 ms (1692397057190 ms)
		0,  // 1692397060000 ms (1692397067190 ms)
		99, // 1692397070000 ms (1692397077190 ms)
		0,  // 1692397080000 ms (1692397087190 ms)
		0,  // 1692397090000 ms (1692397097190 ms)
		0,  // 1692397100000 ms (1692397107190 ms)
		0,  // 1692397110000 ms (1692397117190 ms)
		0,  // 1692397120000 ms (1692397127190 ms)
	}, timeline.Samples)
}

func Test_Backfill_Data_Middle(t *testing.T) {
	const startMs = int64(1692397658567)
	const endMs = int64(1692397778567)
	const stepSec = 10

	points := &typesv1.Series{
		Points: []*typesv1.Point{
			// 1692397658567 ms
			// 1692397668567 ms
			// 1692397678567 ms
			// 1692397688567 ms
			// 1692397698567 ms
			// 1692397708567 ms
			{Timestamp: 1692397718567, Value: 99},
			// 1692397728567 ms
			{Timestamp: 1692397738567, Value: 98},
			// 1692397748567 ms
			// 1692397758567 ms
			// 1692397768567 ms
		},
	}

	timeline := timeline.New(points, startMs, endMs, stepSec)

	const snappedStartMs = int64(1692397650000)
	assert.Equal(t, snappedStartMs/1000, timeline.StartTime)
	assert.Equal(t, []uint64{
		0,  // 1692397650000 ms (1692397658567 ms)
		0,  // 1692397660000 ms (1692397668567 ms)
		0,  // 1692397670000 ms (1692397678567 ms)
		0,  // 1692397680000 ms (1692397688567 ms)
		0,  // 1692397690000 ms (1692397698567 ms)
		0,  // 1692397700000 ms (1692397708567 ms)
		99, // 1692397710000 ms (1692397718567 ms)
		0,  // 1692397720000 ms (1692397728567 ms)
		98, // 1692397730000 ms (1692397738567 ms)
		0,  // 1692397740000 ms (1692397748567 ms)
		0,  // 1692397750000 ms (1692397758567 ms)
		0,  // 1692397760000 ms (1692397768567 ms)
	}, timeline.Samples)
}

func Test_Backfill_All(t *testing.T) {
	const startMs = int64(1692398026941)
	const endMs = int64(1692398146941)
	const stepSec = 10

	points := &typesv1.Series{
		Points: []*typesv1.Point{},
	}

	timeline := timeline.New(points, startMs, endMs, stepSec)

	const snappedStartMs = int64(1692398020000)
	assert.Equal(t, snappedStartMs/1000, timeline.StartTime)
	assert.Equal(t, []uint64{
		0, // 1692398020000 ms (1692398026941 ms)
		0, // 1692398030000 ms (1692398036941 ms)
		0, // 1692398040000 ms (1692398046941 ms)
		0, // 1692398050000 ms (1692398056941 ms)
		0, // 1692398060000 ms (1692398066941 ms)
		0, // 1692398070000 ms (1692398076941 ms)
		0, // 1692398080000 ms (1692398086941 ms)
		0, // 1692398090000 ms (1692398096941 ms)
		0, // 1692398100000 ms (1692398106941 ms)
		0, // 1692398110000 ms (1692398116941 ms)
		0, // 1692398120000 ms (1692398126941 ms)
		0, // 1692398130000 ms (1692398136941 ms)
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

func Test_Backfill_LastSample(t *testing.T) {
	const startMs = int64(3000)
	const stepSec = int64(10)
	const snappedStartMs = int64(0)

	series := &typesv1.Series{
		Points: []*typesv1.Point{
			{Timestamp: 23000, Value: 69},
			{Timestamp: 53000, Value: 91},
		},
	}

	t.Run("series value in last step bucket is not included", func(t *testing.T) {
		const endMs = int64(53000)
		tl := timeline.New(series, startMs, endMs, stepSec)

		assert.Equal(t, snappedStartMs/1000, tl.StartTime)
		assert.Equal(t, []uint64{
			0,  //  3000 ms (    0 ms)
			0,  // 13000 ms (10000 ms)
			69, // 23000 ms (20000 ms)
			0,  // 33000 ms (30000 ms)
			0,  // 43000 ms (40000 ms)
		}, tl.Samples)
	})

	t.Run("last bucket does not get backfilled", func(t *testing.T) {
		const endMs = int64(63000)
		tl := timeline.New(series, startMs, endMs, stepSec)

		assert.Equal(t, snappedStartMs/1000, tl.StartTime)
		assert.Equal(t, []uint64{
			0,  //  3000 (    0 ms)
			0,  // 13000 (10000 ms)
			69, // 23000 (20000 ms)
			0,  // 33000 (30000 ms)
			0,  // 43000 (40000 ms)
			91, // 53000 (50000 ms)
		}, tl.Samples)
	})
}

func Test_Timeline_Bounds(t *testing.T) {
	const stepSec = int64(1)
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

	t.Run("start bounded", func(t *testing.T) {
		const startMs = int64(1000)
		const endMs = int64(10_000)

		tl := timeline.New(series, startMs, endMs, stepSec)
		assert.Equal(t, startMs/1000, tl.StartTime)

		assert.Equal(t, []uint64{
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
	})

	t.Run("end bounded", func(t *testing.T) {
		const startMs = int64(0)
		const endMs = int64(9000)

		tl := timeline.New(series, startMs, endMs, stepSec)
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
		}, tl.Samples)
	})

	t.Run("start and end bounded", func(t *testing.T) {
		const startMs = int64(1000)
		const endMs = int64(9000)

		tl := timeline.New(series, startMs, endMs, stepSec)
		assert.Equal(t, startMs/1000, tl.StartTime)

		assert.Equal(t, []uint64{
			0,  // 1000 ms
			69, // 2000 ms
			83, // 3000 ms
			0,  // 4000 ms
			0,  // 5000 ms
			85, // 6000 ms
			0,  // 7000 ms
			91, // 8000 ms
		}, tl.Samples)
	})

	t.Run("start == end", func(t *testing.T) {
		const startMs = int64(1000)
		const endMs = startMs

		tl := timeline.New(series, startMs, endMs, stepSec)
		assert.Equal(t, startMs/1000, tl.StartTime)

		assert.Equal(t, []uint64{}, tl.Samples)
	})

	t.Run("start > end", func(t *testing.T) {
		const startMs = int64(9000)
		const endMs = int64(1000)

		tl := timeline.New(series, startMs, endMs, stepSec)
		assert.Equal(t, startMs/1000, tl.StartTime)

		assert.Equal(t, []uint64{}, tl.Samples)
	})

	t.Run("points are not contained within start and end", func(t *testing.T) {
		const startMs = int64(10_000)
		const endMs = int64(20_000)

		tl := timeline.New(series, startMs, endMs, stepSec)
		assert.Equal(t, startMs/1000, tl.StartTime)

		assert.Equal(t, []uint64{
			0, // 10000 ms
			0, // 11000 ms
			0, // 12000 ms
			0, // 13000 ms
			0, // 14000 ms
			0, // 15000 ms
			0, // 16000 ms
			0, // 17000 ms
			0, // 18000 ms
			0, // 19000 ms
		}, tl.Samples)
	})

	t.Run("start is halfway through a bucket window", func(t *testing.T) {
		const startMs = int64(500)
		const endMs = int64(9000)

		tl := timeline.New(series, startMs, endMs, stepSec)
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
		}, tl.Samples)
	})

	t.Run("end is halfway through a bucket window", func(t *testing.T) {
		const startMs = int64(0)
		const endMs = int64(8500)

		tl := timeline.New(series, startMs, endMs, stepSec)
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
		}, tl.Samples)
	})
}
