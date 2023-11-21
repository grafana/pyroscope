package phlaredb

import (
	"testing"

	"github.com/grafana/pyroscope/pkg/pprof/testhelper"
)

func TestSelectProfiles(t *testing.T) {
	newBlock(t, func() []*testhelper.ProfileBuilder { return nil })
}
