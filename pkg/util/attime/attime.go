package attime

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// this code is a rough copy of this code https://github.com/graphite-project/graphite-web/blob/master/webapp/graphite/render/attime.py

var digitsOnly *regexp.Regexp

func init() {
	digitsOnly = regexp.MustCompile("^\\d+$")
}

func Parse(s string) time.Time {
	s = strings.TrimSpace(s)
	// s = strings.ToLower(s)
	s = strings.Replace(s, "_", "", -1)
	s = strings.Replace(s, ",", "", -1)
	s = strings.Replace(s, " ", "", -1)

	if digitsOnly.MatchString(s) {
		t, _ := time.Parse("20060102", s)
		if len(s) == 8 && t.Year() > 1900 && t.Month() < 13 && t.Day() < 32 {
			return t
		}

		v, _ := strconv.Atoi(s)
		return time.Unix(int64(v), 0)
	}

	ref := s
	offset := ""

	if i := strings.Index(s, "+"); i != -1 {
		ref = s[:i]
		offset = s[i:]
	} else if i := strings.Index(s, "-"); i != -1 {
		ref = s[:i]
		offset = s[i:]
	}

	return parseTimeReference(ref).Add(parseTimeOffset(offset))
}

func parseTimeReference(_ string) time.Time {
	now := time.Now()
	// TODO: implement
	return now
}

func parseTimeOffset(offset string) (d time.Duration) {
	if offset == "" {
		return 0
	}

	sign := 1

	if offset[0] == '-' {
		sign = -1
		offset = offset[1:]
	} else if offset[0] == '+' {
		offset = offset[1:]
	}

	for offset != "" {
		// TODO: this is kind of ugly IMO, I bet it could be refactored to look better
		i := 1
		for i <= len(offset) && digitsOnly.MatchString(offset[:i]) {
			i++
		}
		num, _ := strconv.Atoi(offset[:i-1])
		offset = offset[i-1:]

		i = 1
		for i <= len(offset) && !digitsOnly.MatchString(offset[:i]) {
			i++
		}
		unit := offset[:i-1]
		offset = offset[i-1:]

		d += time.Second * time.Duration(num*sign*getUnitMultiplier(unit))
	}

	return
}

func getUnitMultiplier(s string) int {
	if strings.HasPrefix(s, "s") {
		return 1
	}
	if strings.HasPrefix(s, "min") || strings.HasPrefix(s, "m") {
		return 60
	}
	if strings.HasPrefix(s, "h") {
		return 3600
	}
	if strings.HasPrefix(s, "d") {
		return 3600 * 24
	}
	if strings.HasPrefix(s, "w") {
		return 3600 * 24 * 7
	}
	if strings.HasPrefix(s, "mon") || strings.HasPrefix(s, "M") {
		return 3600 * 24 * 30
	}
	if strings.HasPrefix(s, "y") {
		return 3600 * 24 * 365
	}
	return 0
}
