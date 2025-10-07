package v2

import (
	"math"
	"time"
)

// msToTime converts milliseconds since epoch to time.Time
func msToTime(ms int64) time.Time {
	return time.UnixMilli(ms)
}

// formatDuration formats a duration in minutes to a human-readable string
func formatDuration(minutes int) string {
	d := time.Duration(minutes) * time.Minute
	return d.Round(time.Minute).String()
}

// durationInMinutes calculates duration between two times in minutes
func durationInMinutes(start, end time.Time) int {
	return int(math.Round(end.Sub(start).Minutes()))
}
