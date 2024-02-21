package pprof

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_FixGoProfile(t *testing.T) {
	p, err := OpenFile("testdata/heap_go1.21.pprof")
	require.NoError(t, err)

	f := FixGoProfile(p.Profile)
	s := make(map[string]struct{})
	for _, x := range f.StringTable {
		if _, ok := s[x]; !ok {
			s[x] = struct{}{}
		} else {
			t.Fatal("duplicate string found")
		}
	}

	t.Logf(" * Sample:   %6d -> %-6d", len(p.Sample), len(f.Sample))
	t.Logf(" * Location: %6d -> %-6d", len(p.Location), len(f.Location))
	t.Logf(" * Function: %6d -> %-6d", len(p.Function), len(f.Function))
	t.Logf(" * Strings:  %6d -> %-6d", len(p.StringTable), len(f.StringTable))
	// fix_test.go:24:  * Sample:     6785 -> 3797
	// fix_test.go:25:  * Location:   4848 -> 4680
	// fix_test.go:26:  * Function:   2801 -> 2724
	// fix_test.go:27:  * Strings:    3536 -> 3458
	assert.Equal(t, 2988, len(p.Sample)-len(f.Sample))
	assert.Equal(t, 168, len(p.Location)-len(f.Location))
	assert.Equal(t, 77, len(p.Function)-len(f.Function))
	assert.Equal(t, 78, len(p.StringTable)-len(f.StringTable))
}
