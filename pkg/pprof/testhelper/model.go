package testhelper

import (
	"time"

	"github.com/google/pprof/profile"
)

var (
	FooMapping = &profile.Mapping{
		ID:              1,
		Start:           uint64(1),
		Limit:           uint64(2),
		Offset:          uint64(3),
		File:            "foobar.so",
		BuildID:         "v1",
		HasFunctions:    true,
		HasFilenames:    true,
		HasLineNumbers:  true,
		HasInlineFrames: false,
	}
	FooFunction = &profile.Function{
		ID:         1,
		Name:       "foo",
		SystemName: "sfoo",
		Filename:   "foo.go",
		StartLine:  1,
	}
	FooLocation = &profile.Location{
		ID:       1,
		Address:  1,
		IsFolded: false,
		Mapping:  FooMapping,
		Line: []profile.Line{
			{
				Function: FooFunction,
				Line:     100,
			},
		},
	}
	BarFunction = &profile.Function{
		ID:         2,
		Name:       "bar",
		SystemName: "sbar",
		Filename:   "bar.go",
		StartLine:  1,
	}
	BarLocation = &profile.Location{
		ID:       2,
		Address:  2,
		IsFolded: true,
		Mapping:  FooMapping,
		Line: []profile.Line{
			{
				Function: BarFunction,
				Line:     200,
			},
		},
	}

	FooBarProfile = &profile.Profile{
		SampleType: []*profile.ValueType{
			{Type: "cpu", Unit: "nanoseconds"},
		},
		DefaultSampleType: "cpu",
		Sample: []*profile.Sample{
			{
				Value:    []int64{1},
				Location: []*profile.Location{FooLocation, BarLocation},
			},
			{
				Value:    []int64{2},
				Location: []*profile.Location{FooLocation},
			},
			{
				Value:    []int64{3},
				Location: []*profile.Location{BarLocation},
			},
		},
		TimeNanos:     1,
		DurationNanos: int64(15 * time.Nanosecond),
		Mapping:       []*profile.Mapping{FooMapping},
		Location:      []*profile.Location{FooLocation, BarLocation},
		Function:      []*profile.Function{FooFunction, BarFunction},
		PeriodType: &profile.ValueType{
			Type: "cpu",
			Unit: "nanoseconds",
		},
		Period: 10000000,
	}
)
