package ruby

import (
	"os"
	"testing"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/stretchr/testify/require"
)

func TestPrintRubyProf(t *testing.T) {
	// t.Skip("This test are only useful to discover ruby profile data")
	data, err := os.ReadFile("testdata/rbspy.pprof.gz")
	require.NoError(t, err)
	err = pprof.FromBytes(data, func(p *googlev1.Profile, size int) error {
		for _, l := range p.Location {
			funcId := l.Line[0].FunctionId
			funcName := p.StringTable[p.Function[funcId-1].Name]
			funcFileName := p.StringTable[p.Function[funcId-1].Filename]
			functStartLine := p.Function[funcId-1].StartLine
			t.Logf("Line: %d StartLine: %d Function: %s FileName: %s", l.Line[0].Line, functStartLine, funcName, funcFileName)
		}

		return nil
	})
	require.NoError(t, err)
}
