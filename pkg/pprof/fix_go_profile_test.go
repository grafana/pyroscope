package pprof

import (
	"bufio"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_FixGoProfile(t *testing.T) {
	p, err := OpenFile("testdata/gotruncatefix/heap_go_truncated_4.pb.gz")
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

func Test_dropGoTypeParameters(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no type parameters",
			input:    "github.com/grafana/pyroscope/pkg/distributor.(*Distributor).Push",
			expected: "github.com/grafana/pyroscope/pkg/distributor.(*Distributor).Push",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "simple type parameter",
			input:    "pkg.Func[go.shape.int]",
			expected: "pkg.Func",
		},
		{
			name:     "type parameter with suffix",
			input:    "pkg.(*T[go.shape.int]).Method",
			expected: "pkg.(*T).Method",
		},
		{
			name:     "multiple type parameters",
			input:    "pkg.Func[go.shape.int,go.shape.string]",
			expected: "pkg.Func",
		},
		{
			name:     "nested brackets in type parameter",
			input:    "pkg.(*T[go.shape.struct { F []uint8 }]).Method",
			expected: "pkg.(*T).Method",
		},
		{
			name:     "bracket without go.shape prefix",
			input:    "pkg.Func[int]",
			expected: "pkg.Func[int]",
		},
		{
			name:     "go.shape prefix without opening bracket",
			input:    "go.shape.int",
			expected: "go.shape.int",
		},
		{
			name:     "multiple separate type parameter sections",
			input:    "pkg.Func[go.shape.int].Method[go.shape.string]",
			expected: "pkg.Func.Method",
		},
		{
			name:     "multiple type parameter sections with nested brackets",
			input:    "pkg.Func[go.shape.struct { F []uint8 }].Method[go.shape.int]",
			expected: "pkg.Func.Method",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, dropGoTypeParameters(tt.input))
		})
	}
}

func Benchmark_dropGoTypeParameters(b *testing.B) {
	withParams := `github.com/bufbuild/connect-go.(*Client[go.shape.struct { github.com/grafana/pyroscope/api/gen/proto/go/push/v1.state google.golang.org/protobuf/internal/impl.MessageState; github.com/grafana/pyroscope/api/gen/proto/go/push/v1.sizeCache int32; github.com/grafana/pyroscope/api/gen/proto/go/push/v1.unknownFields []uint8; Series []*github.com/grafana/pyroscope/api/gen/proto/go/push/v1.RawProfileSeries "protobuf:\"bytes,1,rep,name=series,proto3\" json:\"series,omitempty\"" },go.shape.struct { github.com/grafana/pyroscope/api/gen/proto/go/push/v1.state google.golang.org/protobuf/internal/impl.MessageState; github.com/grafana/pyroscope/api/gen/proto/go/push/v1.sizeCache int32; github.com/grafana/pyroscope/api/gen/proto/go/push/v1.unknownFields []uint8 }]).CallUnary`
	withoutParams := "github.com/grafana/pyroscope/pkg/distributor.(*Distributor).Push"

	b.Run("with_type_params", func(b *testing.B) {
		for b.Loop() {
			dropGoTypeParameters(withParams)
		}
	})
	b.Run("without_type_params", func(b *testing.B) {
		for b.Loop() {
			dropGoTypeParameters(withoutParams)
		}
	})
}
