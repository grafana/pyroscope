package store

import (
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid"
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

// CreatePartitionKey creates a partition key for a block. It is meant to be used for newly inserted blocks, as it relies
// on the index's currently configured partition duration to create the key.
//
// Note: Using this for existing blocks following a partition duration change can produce the wrong key. Callers should
// verify that the returned partition actually contains the block.
func CreatePartitionKey(blockId string, dur time.Duration) PartitionKey {
	t := ulid.Time(ulid.MustParse(blockId).Time()).UTC()

	var b strings.Builder
	b.Grow(16)

	year, month, day := t.Date()
	b.WriteString(fmt.Sprintf("%04d%02d%02d", year, month, day))

	partitionDuration := dur
	if partitionDuration < 24*time.Hour {
		hour := (t.Hour() / int(partitionDuration.Hours())) * int(partitionDuration.Hours())
		b.WriteString(fmt.Sprintf("T%02d", hour))
	}

	mDuration := model.Duration(partitionDuration)
	b.WriteString(".")
	b.WriteString(mDuration.String())

	return PartitionKey(b.String())
}

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
