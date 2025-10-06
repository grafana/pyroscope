package v2

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"time"
)

// parseTime parses a time string that can be in RFC3339 format or relative format like "now-24h"
func parseTime(t string) (time.Time, error) {
	// Check if it's relative format (now-XXh or now-XXm)
	if matched, _ := regexp.MatchString(`^now(-\d+[smhd])?$`, t); matched {
		duration := time.Duration(0)
		if t != "now" {
			re := regexp.MustCompile(`^now-(\d+)([smhd])$`)
			matches := re.FindStringSubmatch(t)
			if len(matches) == 3 {
				value, _ := strconv.Atoi(matches[1])
				unit := matches[2]
				switch unit {
				case "s":
					duration = time.Duration(value) * time.Second
				case "m":
					duration = time.Duration(value) * time.Minute
				case "h":
					duration = time.Duration(value) * time.Hour
				case "d":
					duration = time.Duration(value) * 24 * time.Hour
				}
			}
		}
		return time.Now().Add(-duration), nil
	}

	// Try RFC3339
	parsed, err := time.Parse(time.RFC3339, t)
	if err == nil {
		return parsed, nil
	}

	// Try Unix timestamp
	if unix, err := strconv.ParseInt(t, 10, 64); err == nil {
		return time.Unix(unix, 0), nil
	}

	return time.Time{}, fmt.Errorf("unable to parse time: %s", t)
}

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
