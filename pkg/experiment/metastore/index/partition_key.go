package index

import (
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/common/model"
)

const (
	dayLayout  = "20060102"
	hourLayout = "20060102T15"
)

func getTimeLayout(d model.Duration) string {
	if time.Duration(d) >= 24*time.Hour {
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
	mDur, err := model.ParseDuration(parts[1])
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("invalid duration in partition key: %s", k)
	}
	t, err = time.Parse(getTimeLayout(mDur), parts[0])
	return t, time.Duration(mDur), err
}
