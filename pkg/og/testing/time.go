package testing

import "time"

var format = "2006-01-02-15:04:05.999999999 MST"

func ParseTime(str string) time.Time {
	r, err := time.Parse(format, str+" UTC")
	if err != nil {
		panic(err)
	}
	return r.UTC()
}

func SimpleTime(i int) time.Time {
	return time.Time{}.Add(time.Duration(i) * time.Second).UTC()
}

func SimpleUTime(i int) time.Time {
	return time.Unix(int64(i), 0)
}

func PrintTime(t time.Time) int {
	return int(t.Sub(time.Time{}) / time.Second)
}
