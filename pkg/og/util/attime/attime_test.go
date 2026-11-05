package attime

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Run("simple cases", func(t *testing.T) {
		now := time.Unix(1577836800, 0)
		timeNow = func() time.Time { return now }
		t.Cleanup(func() { timeNow = time.Now })

		require.Equal(t, now, Parse("now"))
		require.Equal(t, now.Add(-1*time.Second), Parse("now-1s"))
		require.Equal(t, now.Add(1*time.Second), Parse("now+1s"))
		require.Equal(t, now.Add(-1*time.Minute), Parse("now-1min"))
		require.Equal(t, now.Add(-1*time.Hour), Parse("now-1h"))
		require.Equal(t, now.Add(-1*time.Hour*24), Parse("now-1d"))
		require.Equal(t, now.Add(-1*time.Hour*24*7), Parse("now-1w"))
		require.Equal(t, now.Add(-1*time.Hour*24*30), Parse("now-1mon"))
		require.Equal(t, now.Add(-1*time.Hour*24*30), Parse("now-1M"))
		require.Equal(t, now.Add(-1*time.Hour*24*365), Parse("now-1y"))
		require.Equal(t, now, Parse("now-1"))
		require.Equal(t, time.Unix(1577836800, 0).UTC(), Parse("20200101"))
		require.Equal(t, time.Unix(1577836800, 0), Parse("1577836800"))
		require.Equal(t, time.Unix(1577836800, 1000000), Parse("1577836800001"))
		require.Equal(t, time.Unix(1577836800, 1000), Parse("1577836800000001"))
		require.Equal(t, time.Unix(1577836800, 1), Parse("1577836800000000001"))
	})
}
