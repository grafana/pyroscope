package index

import (
	"fmt"
	"strings"
	"time"
)

const (
	dayLayout  = "20060102"
	hourLayout = "20060102T15"
)

func getTimeLayout(d time.Duration) string {
	if d >= 24*time.Hour {
		return dayLayout
	} else {
		return hourLayout
	}
}

type PartitionKey string

func (k PartitionKey) Parse() (t time.Time, d time.Duration, err error) {
	parts := strings.Split(string(k), ".")
	if len(parts) != 2 {
		return time.Time{}, 0, fmt.Errorf("invalid partition key: %s", k)
	}
	d, err = time.ParseDuration(parts[1])
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("invalid duration in partition key: %s", k)
	}
	t, err = time.Parse(getTimeLayout(d), parts[0])
	return t, d, err
}

func (k PartitionKey) compare(other PartitionKey) int {
	if k == other {
		return 0
	}
	tSelf, _, err := k.Parse()
	if err != nil {
		return strings.Compare(string(k), string(other))
	}
	tOther, _, err := other.Parse()
	if err != nil {
		return strings.Compare(string(k), string(other))
	}
	return tSelf.Compare(tOther)
}

func (k PartitionKey) inRange(start, end int64) bool {
	pStart, d, err := k.Parse()
	if err != nil {
		return false
	}
	pEnd := pStart.Add(d)
	return start < pEnd.UnixMilli() && end > pStart.UnixMilli()
}
