package segment

import (
	"time"
)

type Timeline struct {
	st                      time.Time
	et                      time.Time
	StartTime               int64    `json:"startTime"`
	Samples                 []uint64 `json:"samples"`
	durationDelta           time.Duration
	DurationDeltaNormalized int64 `json:"durationDelta"`

	// Watermarks map contains down-sampling watermarks (Unix timestamps)
	// describing resolution levels of the timeline.
	//
	// Resolution in seconds is calculated as 10^k, where k is the map key.
	// Meaning that any range within these 10^k seconds contains not more
	// than one sample. Any sub-range less than 10^k shows down-sampled data.
	//
	// Given the map:
	//  1: 1635508310
	//  2: 1635507500
	//  3: 1635506200
	//
	// This should be read as follows:
	//  1. Data after 1635508310 is as precise as possible (10s resolution),
	//     down-sampling was not applied.
	//  2. Data before 1635508310 has resolution 100s
	//  3. Data before 1635507500 has resolution 1000s
	//  4. Data before 1635506200 has resolution 10000s
	Watermarks map[int]int64 `json:"watermarks"`
}
