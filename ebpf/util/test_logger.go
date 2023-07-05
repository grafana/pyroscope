package util

import (
	"os"
	"testing"
	"time"

	"github.com/go-kit/log"
)

// TestLogger generates a logger for a test.
func TestLogger(t *testing.T) log.Logger {
	t.Helper()

	l := log.NewSyncLogger(log.NewLogfmtLogger(os.Stderr))
	l = log.WithPrefix(l,
		"test", t.Name(),
		"ts", log.Valuer(testTimestamp),
	)

	return l
}

// testTimestamp is a log.Valuer that returns the timestamp
// without the date or timezone, reducing the noise in the test.
func testTimestamp() interface{} {
	t := time.Now().UTC()
	return t.Format("15:04:05.000")
}
