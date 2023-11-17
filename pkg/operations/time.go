package operations

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/common/model"
)

func ParseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return t, nil
	}

	// try if it is a relative time
	d, rerr := parseRelativeTime(s)
	if rerr == nil {
		return time.Now().Add(-d), nil
	}

	timestamp, terr := strconv.ParseInt(s, 10, 64)
	if terr == nil {
		/**
		1689341454
		1689341454046
		1689341454046908
		1689341454046908187
		*/
		switch len(s) {
		case 10:
			return time.Unix(timestamp, 0), nil
		case 13:
			return time.UnixMilli(timestamp), nil
		case 16:
			return time.UnixMicro(timestamp), nil
		case 19:
			return time.Unix(0, timestamp), nil
		default:
			return time.Time{}, fmt.Errorf("invalid timestamp length: %s", s)
		}
	}
	// if not return first error
	return time.Time{}, err

}

func parseRelativeTime(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "now" {
		return 0, nil
	}
	s = strings.TrimPrefix(s, "now-")

	d, err := model.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	return time.Duration(d), nil
}
