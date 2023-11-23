package operations

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseTime(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		want    time.Time
		wantErr bool
	}{
		{name: "relative time days", args: "now-7d", want: time.Now().Add(-24 * 7 * time.Hour)},
		{name: "relative time hours", args: "now-24h", want: time.Now().Add(-24 * time.Hour)},
		{name: "relative time minutes", args: "now-2h30m", want: time.Now().Add(-2 * time.Hour).Add(-30 * time.Minute)},
		{name: "relative time seconds", args: "now-2h30m45s", want: time.Now().Add(-2 * time.Hour).Add(-30 * time.Minute).Add(-45 * time.Second)},
		{name: "relative time wrong format", args: "24h-ago", wantErr: true},
		{name: "absolute time utc", args: "2006-01-02T15:04:05Z", want: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC)},
		{name: "absolute time utc with positive offset", args: "2006-01-02T15:04:05+04:00", want: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC).Add(-4 * time.Hour)},
		{name: "absolute time utc with negative offset", args: "2006-01-02T15:04:05-04:00", want: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC).Add(4 * time.Hour)},
		{name: "absolute time wrong format", args: "2006-01-02", wantErr: true},
		{name: "timestamp seconds", args: "1689341454", want: time.Date(2023, 7, 14, 13, 30, 54, 0, time.UTC)},
		{name: "timestamp milli", args: "1689341454046", want: time.Date(2023, 7, 14, 13, 30, 54, 0, time.UTC)},
		{name: "timestamp micro", args: "1689341454046908", want: time.Date(2023, 7, 14, 13, 30, 54, 0, time.UTC)},
		{name: "timestamp nano", args: "1689341454046908187", want: time.Date(2023, 7, 14, 13, 30, 54, 0, time.UTC)},
		{name: "timestamp wrong format", args: "16893", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTime(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTime() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.WithinDuration(t, tt.want, got, 1*time.Second)
		})
	}
}
