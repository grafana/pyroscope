package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"time"

	pprofprofile "github.com/google/pprof/profile"
)

// Keep the seeded profile comfortably below Pyroscope's default limits of
// 16,000 samples and 4 MiB uncompressed. Sampling across the full input, rather
// than taking a prefix, retains a representative spread of the real profile.
const smokeProfileMaxSamples = 4_096

//go:embed testdata/profile.pprof
var smokeProfileBytes []byte

func buildSmokeProfile(profileTime time.Time) ([]byte, error) {
	profile, err := pprofprofile.ParseData(smokeProfileBytes)
	if err != nil {
		return nil, fmt.Errorf("parse embedded profile: %w", err)
	}

	if len(profile.Sample) > smokeProfileMaxSamples {
		samples := make([]*pprofprofile.Sample, smokeProfileMaxSamples)
		for i := range samples {
			samples[i] = profile.Sample[i*len(profile.Sample)/len(samples)]
		}
		profile.Sample = samples
		profile = profile.Compact()
	}
	profile.TimeNanos = profileTime.UnixNano()

	if err := profile.CheckValid(); err != nil {
		return nil, fmt.Errorf("validate embedded profile: %w", err)
	}

	var buf bytes.Buffer
	if err := profile.Write(&buf); err != nil {
		return nil, fmt.Errorf("encode embedded profile: %w", err)
	}
	return buf.Bytes(), nil
}
