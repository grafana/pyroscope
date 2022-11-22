package treesvg

import (
	"github.com/google/pprof/driver"
	"github.com/google/pprof/profile"
)

type sym struct {
}

func (_ *sym) Symbolize(mode string, srcs driver.MappingSources, prof *profile.Profile) error {
	return nil
}
