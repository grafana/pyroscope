package treesvg

import (
	"time"

	"github.com/google/pprof/profile"
)

type fetch struct {
	profile *profile.Profile
}

func (f *fetch) Fetch(src string, duration, timeout time.Duration) (*profile.Profile, string, error) {
	return f.profile, "", nil
}
