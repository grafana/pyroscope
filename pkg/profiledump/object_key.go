package profiledump

import (
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
)

const ObjectPrefix = "profile-debug-dumps/"

// Format identifies the native format, independently of compression.
type Format string

const FormatPprof Format = "pprof"

func parseCaptureID(s string) (ulid.ULID, error) {
	id, err := ulid.ParseStrict(s)
	if err != nil {
		return ulid.ULID{}, fmt.Errorf("invalid capture ID: %w", err)
	}
	if id.String() != s {
		return ulid.ULID{}, fmt.Errorf("noncanonical capture ID")
	}
	return id, nil
}

func validateCaptureTime(t time.Time) error {
	if t.UTC().Year() < 1970 || t.UTC().Year() > 9999 {
		return fmt.Errorf("capture time must be within years 1970 through 9999 UTC")
	}
	return nil
}
