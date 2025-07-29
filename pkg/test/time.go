package test

import (
	"crypto/rand"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/prometheus/common/model"
)

func ULID(t string) string {
	parsed, _ := time.Parse(time.RFC3339, t)
	l := ulid.MustNew(ulid.Timestamp(parsed), rand.Reader)
	return l.String()
}

func UnixMilli(t string) int64 {
	return Time(t).UnixMilli()
}

func Time(t string) time.Time {
	x, _ := time.Parse(time.RFC3339, t)
	return x
}

func Duration(d string) time.Duration {
	parsed, _ := model.ParseDuration(d)
	return time.Duration(parsed)
}
