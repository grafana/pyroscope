package convert

import (
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
)

func readFile(path string) []byte {
	f, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return f
}

func requireConverter(t *testing.T, m ProfileFile, expected any) {
	t.Helper()

	f, err := converter(m)
	require.NoError(t, err)
	require.NotNil(t, f)
	require.Equal(t, reflect.ValueOf(expected).Pointer(), reflect.ValueOf(f).Pointer())
}

func TestConverterDetectingFormat(t *testing.T) {
	perfScriptData := []byte("java 12688 [002] 6544038.708352: cpu-clock:\n\n")

	tests := []struct {
		name     string
		file     ProfileFile
		expected any
		wantErr  bool
	}{
		// pprof by type
		{name: "pprof type only", file: ProfileFile{Type: "pprof"}, expected: PprofToProfile},
		{name: "pprof type over json filename", file: ProfileFile{Name: "profile.json", Type: "pprof"}, expected: PprofToProfile},
		{name: "pprof type over json content", file: ProfileFile{Data: []byte(`{"flamebearer":""}`), Type: "pprof"}, expected: PprofToProfile},
		// pprof by filename
		{name: "pprof filename over json content", file: ProfileFile{Name: "profile.pprof", Data: []byte(`{"flamebearer":""}`)}, expected: PprofToProfile},
		{name: "pprof filename ignores unsupported type", file: ProfileFile{Name: "profile.pprof", Type: "unsupported"}, expected: PprofToProfile},
		// pprof by content
		{name: "pprof uncompressed content", file: ProfileFile{Data: []byte{0x0a, 0x04}}, expected: PprofToProfile},
		{name: "pprof gzip content", file: ProfileFile{Data: []byte{0x1f, 0x8b}}, expected: PprofToProfile},
		{name: "pprof gzip ignores unsupported type", file: ProfileFile{Data: []byte{0x1f, 0x8b}, Type: "unsupported"}, expected: PprofToProfile},
		{name: "pprof gzip ignores unsupported filename", file: ProfileFile{Name: "profile.unsupported", Data: []byte{0x1f, 0x8b}}, expected: PprofToProfile},
		// json by type
		{name: "json type only", file: ProfileFile{Type: "json"}, expected: JSONToProfile},
		{name: "json type over pprof filename", file: ProfileFile{Name: "profile.pprof", Type: "json"}, expected: JSONToProfile},
		{name: "json type over gzip content", file: ProfileFile{Data: []byte{0x1f, 0x8b}, Type: "json"}, expected: JSONToProfile},
		// json by filename
		{name: "json filename over gzip content", file: ProfileFile{Name: "profile.json", Data: []byte{0x1f, 0x8b}}, expected: JSONToProfile},
		{name: "json filename ignores unsupported type", file: ProfileFile{Name: "profile.json", Type: "unsupported"}, expected: JSONToProfile},
		// json by content
		{name: "json content", file: ProfileFile{Data: []byte(`{"flamebearer":""}`)}, expected: JSONToProfile},
		{name: "json content ignores unsupported type", file: ProfileFile{Data: []byte(`{"flamebearer":""}`), Type: "unsupported"}, expected: JSONToProfile},
		{name: "json content ignores unsupported filename", file: ProfileFile{Name: "profile.unsupported", Data: []byte(`{"flamebearer":""}`)}, expected: JSONToProfile},
		// collapsed by type
		{name: "collapsed type only", file: ProfileFile{Type: "collapsed"}, expected: CollapsedToProfile},
		{name: "collapsed type over pprof filename", file: ProfileFile{Name: "profile.pprof", Type: "collapsed"}, expected: CollapsedToProfile},
		{name: "collapsed type over gzip content", file: ProfileFile{Data: []byte{0x1f, 0x8b}, Type: "collapsed"}, expected: CollapsedToProfile},
		// collapsed by filename
		{name: "collapsed filename over gzip content", file: ProfileFile{Name: "profile.collapsed", Data: []byte{0x1f, 0x8b}}, expected: CollapsedToProfile},
		{name: "collapsed filename ignores unsupported type", file: ProfileFile{Name: "profile.collapsed", Type: "unsupported"}, expected: CollapsedToProfile},
		{name: "collapsed txt filename ignores unsupported type", file: ProfileFile{Name: "profile.collapsed.txt", Type: "unsupported"}, expected: CollapsedToProfile},
		// collapsed by content
		{name: "collapsed content", file: ProfileFile{Data: []byte("fn1 1\nfn2 2")}, expected: CollapsedToProfile},
		{name: "collapsed content ignores unsupported type", file: ProfileFile{Data: []byte("fn1 1\nfn2 2"), Type: "unsupported"}, expected: CollapsedToProfile},
		{name: "collapsed content ignores unsupported filename", file: ProfileFile{Name: "profile.unsupported", Data: []byte("fn1 1\nfn2 2")}, expected: CollapsedToProfile},
		// perf script
		{name: "perf script by content", file: ProfileFile{Data: perfScriptData}, expected: PerfScriptToProfile},
		{name: "perf script by txt extension", file: ProfileFile{Name: "foo.txt", Data: perfScriptData}, expected: PerfScriptToProfile},
		{name: "perf script by extension", file: ProfileFile{Name: "foo.perf_script", Data: []byte("foo;bar 239")}, expected: PerfScriptToProfile},
		// error
		{name: "empty profile file", file: ProfileFile{}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr {
				_, err := converter(tt.file)
				require.Error(t, err)
				return
			}
			requireConverter(t, tt.file, tt.expected)
		})
	}
}

func TestConvert(t *testing.T) {
	t.Run("converts malformed pprof", func(t *testing.T) {
		m := ProfileFile{
			Type: "pprof",
			Data: readFile("./testdata/cpu-unknown.pb.gz"),
		}

		f, err := converter(m)
		require.NoError(t, err)
		require.NotNil(t, f)

		b, err := f(m.Data, "appname", Limits{MaxNodes: 1024})
		require.NoError(t, err)
		require.NotNil(t, b)
	})

	t.Run("handles pprof invalid fields gracefully", func(t *testing.T) {
		p := &profilev1.Profile{
			SampleType: []*profilev1.ValueType{
				{Type: 1, Unit: 2},
			},
			Sample: []*profilev1.Sample{
				{LocationId: []uint64{1}, Value: []int64{100}},
			},
			Location: []*profilev1.Location{
				{Id: 1, Address: 0x1000, Line: []*profilev1.Line{{FunctionId: 1, Line: 10}}},
			},
			Function: []*profilev1.Function{
				{Id: 1, Name: 1},
			},
			StringTable: []string{"", "cpu", "count", "main"},
		}

		data, err := proto.Marshal(p)
		require.NoError(t, err)

		m := ProfileFile{
			Type: "pprof",
			Data: data,
		}

		f, err := converter(m)
		require.NoError(t, err)
		require.NotNil(t, f)

		b, err := f(m.Data, "test-profile", Limits{MaxNodes: 1024})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "PeriodType is nil")
		require.Nil(t, b)
	})

	t.Run("JSON prunes tree", func(t *testing.T) {
		m := ProfileFile{
			Type: "json",
			Data: readFile("./testdata/profile.json"),
		}

		f, err := converter(m)
		require.NoError(t, err)
		require.NotNil(t, f)

		b, err := f(m.Data, "appname", Limits{MaxNodes: 1})
		require.NoError(t, err)
		require.NotNil(t, b)

		require.Len(t, b[0].FlamebearerProfileV1.Flamebearer.Levels, 2)
	})
}
