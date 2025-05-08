package pprof

import (
	"bufio"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
)

func Test_FixGoProfile(t *testing.T) {
	p, err := OpenFile("testdata/gotruncatefix/heap_go_truncated_4.pb.gz")
	require.NoError(t, err)

	total := samplesTotal(p.Profile)
	f := FixGoProfile(p.Profile)
	s := make(map[string]struct{})
	for _, x := range f.StringTable {
		if _, ok := s[x]; !ok {
			s[x] = struct{}{}
		} else {
			t.Fatal("duplicate string found")
		}
	}

	// Assert that the total sum of samples does not
	// change compared to the source profile.
	assert.Equal(t, total, samplesTotal(p.Profile))
	assert.Equal(t, total, samplesTotal(f))

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

	// Now we can normalize the profile aggregate samples.
	// This must not affect the total sum of values.
	x := RawFromProto(f)
	x.Normalize()
	assert.Equal(t, total, samplesTotal(x.Profile))
}

func samplesTotal(p *profilev1.Profile) int {
	var total int
	for _, s := range p.Sample {
		total += int(s.Value[0])
	}
	return total
}

func Test_DropGoTypeParameters(t *testing.T) {
	ef, err := os.Open("testdata/go_type_parameters.expected.txt")
	require.NoError(t, err)
	defer ef.Close()

	in, err := os.Open("testdata/go_type_parameters.txt")
	require.NoError(t, err)
	defer in.Close()

	input := bufio.NewScanner(in)
	expected := bufio.NewScanner(ef)
	for input.Scan() {
		expected.Scan()
		require.Equal(t, expected.Text(), dropGoTypeParameters(input.Text()))
	}

	require.NoError(t, input.Err())
	require.NoError(t, expected.Err())
	require.False(t, expected.Scan())
}
