package pprof

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/pprof/profile"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/pkg/testhelper"
)

func Test_Merge_Single(t *testing.T) {
	p, err := OpenFile("testdata/go.cpu.labels.pprof")
	require.NoError(t, err)
	var m ProfileMerge
	require.NoError(t, m.Merge(p.CloneVT(), true))
	sortLabels(p.Profile)
	act := m.Profile()
	exp := p.Profile
	testhelper.EqualProto(t, exp, act)
}

func sortLabels(p *profilev1.Profile) {
	for _, s := range p.Sample {
		sort.Sort(LabelsByKeyValue(s.Label))
	}
}

type fuzzEvent byte

const (
	fuzzEventUnknown fuzzEvent = iota
	fuzzEventPostDecode
	fuzzEventPostMerge
)

type eventSocket struct {
	lck  sync.Mutex
	fMap map[string]net.Conn
}

var eventWriter = &eventSocket{
	fMap: make(map[string]net.Conn),
}

func eventWrite(t *testing.T, msg []byte) {
	eventWriter.lck.Lock()
	c, ok := eventWriter.fMap[eventName(t)]
	if !ok {
		var err error
		c, err = net.Dial("unix", eventPath(t))
		if err != nil {
			eventWriter.lck.Unlock()
			t.Fatalf("error connecting: %v", err)
			return
		}
		eventWriter.fMap[eventName(t)] = c
	}
	eventWriter.lck.Unlock()
	_, err := c.Write(msg)

	require.NoError(t, err)
}

func eventName(t testing.TB) string {
	return strings.Split(t.Name(), "/")[0]
}

func eventPath(t testing.TB) string {
	hash := md5.Sum([]byte(eventName(t)))
	p := filepath.Join(os.TempDir(), hex.EncodeToString(hash[:])+"-fuzz-events.sock")
	return p
}

func isFuzzWorker() bool {
	for _, arg := range os.Args {
		if arg == "-test.fuzzworker" {
			return true
		}
		if arg == "-fuzzworker" {
			return true
		}
	}
	return false
}

// runEventsGatherer starts a server that listens for events from the fuzzing worker processes. This allows us to gather additional metrics on how successful the fuzzing is with finding valid profiles.
func runEventsGatherer(t testing.TB) {
	fPath := eventPath(t)
	_ = os.Remove(fPath)
	socket, err := net.Listen("unix", fPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		socket.Close()
		_ = os.Remove(fPath)
	})

	eventCh := make(chan fuzzEvent, 32)
	go func() {
		for {
			conn, err := socket.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				buf := make([]byte, 1024)
				for {
					n, err := conn.Read(buf)
					if err != nil {
						return
					}
					for _, b := range buf[:n] {
						eventCh <- fuzzEvent(b)
					}
				}
			}(conn)
		}
	}()

	go func() {
		ticker := time.NewTicker(3 * time.Second)
		stdout := os.Stdout
		defer ticker.Stop()
		var totalPostDecode, totalPostMerge int64
		var lastPostDecode, lastPostMerge int64
		for {
			select {
			case <-ticker.C:
				fmt.Fprintf(stdout, "postDecode: %d/%d (last 3s, total) postMerge %d/%d (last 3s, total)\n", totalPostDecode-lastPostDecode, totalPostDecode, totalPostMerge-lastPostMerge, totalPostMerge)
				lastPostDecode = totalPostDecode
				lastPostMerge = totalPostMerge
			case event := <-eventCh:
				switch event {
				case fuzzEventPostDecode:
					totalPostDecode += 1
				case fuzzEventPostMerge:
					totalPostMerge += 1
				}
			}
		}
	}()
}

func Fuzz_Merge_Single(f *testing.F) {
	// setup event handler (only in main process)
	if !isFuzzWorker() {
		runEventsGatherer(f)
	}

	for _, file := range []string{
		"testdata/go.cpu.labels.pprof",
		"testdata/heap",
		"testdata/profile_java",
		"testdata/profile_rust",
	} {
		raw, err := OpenFile(file)
		require.NoError(f, err)
		data, err := raw.MarshalVT()
		require.NoError(f, err)
		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var p profilev1.Profile
		err := p.UnmarshalVT(data)
		if err != nil {
			return
		}

		eventWrite(t, []byte{byte(fuzzEventPostDecode)})
		var m ProfileMerge
		err = m.Merge(&p, true)
		if err != nil {
			return
		}
		eventWrite(t, []byte{byte(fuzzEventPostMerge)})
	})
}

func Test_Merge_Self(t *testing.T) {
	p, err := OpenFile("testdata/go.cpu.labels.pprof")
	require.NoError(t, err)
	var m ProfileMerge
	require.NoError(t, m.Merge(p.CloneVT(), true))
	require.NoError(t, m.Merge(p.CloneVT(), true))
	for i := range p.Sample {
		s := p.Sample[i]
		for j := range s.Value {
			s.Value[j] *= 2
		}
	}
	p.DurationNanos *= 2
	sortLabels(p.Profile)
	testhelper.EqualProto(t, p.Profile, m.Profile())
}

func Test_Merge_Halves(t *testing.T) {
	p, err := OpenFile("testdata/go.cpu.labels.pprof")
	require.NoError(t, err)

	a := p.CloneVT()
	b := p.CloneVT()
	n := len(p.Sample) / 2
	a.Sample = a.Sample[:n]
	b.Sample = b.Sample[n:]

	var m ProfileMerge
	require.NoError(t, m.Merge(a, true))
	require.NoError(t, m.Merge(b, true))

	// Merge with self for normalisation.
	var sm ProfileMerge
	require.NoError(t, sm.Merge(p.CloneVT(), true))
	p.DurationNanos *= 2

	sortLabels(p.Profile)
	testhelper.EqualProto(t, p.Profile, m.Profile())
}

func Test_Merge_Sample(t *testing.T) {
	stringTable := []string{
		"",
		"samples",
		"count",
		"cpu",
		"nanoseconds",
		"foo",
		"bar",
		"profile_id",
		"c717c11b87121639",
		"function",
		"slow",
		"8c946fa4ae322f7f",
		"fast",
		"main.work",
		"/Users/kolesnikovae/Documents/src/pyroscope/examples/golang-push/simple/main.go",
		"main.slowFunction.func1",
		"runtime/pprof.Do",
		"/usr/local/go/src/runtime/pprof/runtime.go",
		"main.slowFunction",
		"main.main.func2",
		"github.com/pyroscope-io/client/pyroscope.TagWrapper.func1",
		"/Users/kolesnikovae/go/pkg/mod/github.com/pyroscope-io/client@v0.2.4-0.20220607180407-0ba26860ce5b/pyroscope/api.go",
		"github.com/pyroscope-io/client/pyroscope.TagWrapper",
		"main.main",
		"runtime.main",
		"/usr/local/go/src/runtime/proc.go",
		"main.fastFunction.func1",
		"main.fastFunction",
	}

	a := &profilev1.Profile{
		SampleType: []*profilev1.ValueType{
			{
				Type: 1,
				Unit: 2,
			},
			{
				Type: 3,
				Unit: 4,
			},
		},
		Sample: []*profilev1.Sample{
			{
				LocationId: []uint64{1, 2, 3},
				Value:      []int64{1, 10000000},
				Label: []*profilev1.Label{
					{Key: 5, Str: 6},
					{Key: 7, Str: 8},
					{Key: 9, Str: 10},
				},
			},
		},
		Mapping: []*profilev1.Mapping{
			{
				Id:           1,
				HasFunctions: true,
			},
		},
		Location: []*profilev1.Location{
			{
				Id:        1,
				MappingId: 1,
				Address:   19497668,
				Line:      []*profilev1.Line{{FunctionId: 1, Line: 19}},
			},
			{
				Id:        2,
				MappingId: 1,
				Address:   19498429,
				Line:      []*profilev1.Line{{FunctionId: 2, Line: 43}},
			},
			{
				Id:        3,
				MappingId: 1,
				Address:   19267106,
				Line:      []*profilev1.Line{{FunctionId: 3, Line: 40}},
			},
		},
		Function: []*profilev1.Function{
			{
				Id:         1,
				Name:       13,
				SystemName: 13,
				Filename:   14,
			},
			{
				Id:         2,
				Name:       15,
				SystemName: 15,
				Filename:   14,
			},
			{
				Id:         3,
				Name:       16,
				SystemName: 16,
				Filename:   17,
			},
		},
		StringTable:   stringTable,
		TimeNanos:     1654798932062349000,
		DurationNanos: 10123363553,
		PeriodType: &profilev1.ValueType{
			Type: 3,
			Unit: 4,
		},
		Period: 10000000,
	}

	b := &profilev1.Profile{
		SampleType: []*profilev1.ValueType{
			{
				Type: 1,
				Unit: 2,
			},
			{
				Type: 3,
				Unit: 4,
			},
		},
		Sample: []*profilev1.Sample{
			{
				LocationId: []uint64{1},
				Value:      []int64{1, 10000000},
				Label: []*profilev1.Label{
					{Key: 5, Str: 6},
					{Key: 7, Str: 11},
					{Key: 9, Str: 12},
				},
			},
			{
				LocationId: []uint64{2, 3, 4}, // Same
				Value:      []int64{1, 10000000},
				Label: []*profilev1.Label{
					{Key: 5, Str: 6},
					{Key: 7, Str: 8},
					{Key: 9, Str: 10},
				},
			},
		},
		Mapping: []*profilev1.Mapping{
			{
				Id:           1,
				HasFunctions: true,
			},
		},
		Location: []*profilev1.Location{
			{
				Id:        1,
				MappingId: 1,
				Address:   19499013,
				Line:      []*profilev1.Line{{FunctionId: 1, Line: 42}},
			},
			{
				Id:        2,
				MappingId: 1,
				Address:   19497668,
				Line:      []*profilev1.Line{{FunctionId: 2, Line: 19}},
			},
			{
				Id:        3,
				MappingId: 1,
				Address:   19498429,
				Line:      []*profilev1.Line{{FunctionId: 3, Line: 43}},
			},
			{
				Id:        4,
				MappingId: 1,
				Address:   19267106,
				Line:      []*profilev1.Line{{FunctionId: 4, Line: 40}},
			},
		},
		Function: []*profilev1.Function{
			{
				Id:         1,
				Name:       18,
				SystemName: 18,
				Filename:   14,
			},
			{
				Id:         2,
				Name:       13,
				SystemName: 13,
				Filename:   14,
			},
			{
				Id:         3,
				Name:       15,
				SystemName: 15,
				Filename:   14,
			},
			{
				Id:         4,
				Name:       16,
				SystemName: 16,
				Filename:   17,
			},
		},
		StringTable:   stringTable,
		TimeNanos:     1654798932062349000,
		DurationNanos: 10123363553,
		PeriodType: &profilev1.ValueType{
			Type: 3,
			Unit: 4,
		},
		Period: 10000000,
	}

	expected := &profilev1.Profile{
		SampleType: []*profilev1.ValueType{
			{
				Type: 1,
				Unit: 2,
			},
			{
				Type: 3,
				Unit: 4,
			},
		},
		Sample: []*profilev1.Sample{
			{
				LocationId: []uint64{1, 2, 3},
				Value:      []int64{2, 20000000},
				Label: []*profilev1.Label{
					{Key: 5, Str: 6},
					{Key: 7, Str: 8},
					{Key: 9, Str: 10},
				},
			},
			{
				LocationId: []uint64{4},
				Value:      []int64{1, 10000000},
				Label: []*profilev1.Label{
					{Key: 5, Str: 6},
					{Key: 7, Str: 11},
					{Key: 9, Str: 12},
				},
			},
		},
		Mapping: []*profilev1.Mapping{
			{
				Id:           1,
				HasFunctions: true,
			},
		},
		Location: []*profilev1.Location{
			{
				Id:        1,
				MappingId: 1,
				Address:   19497668,
				Line:      []*profilev1.Line{{FunctionId: 1, Line: 19}},
			},
			{
				Id:        2,
				MappingId: 1,
				Address:   19498429,
				Line:      []*profilev1.Line{{FunctionId: 2, Line: 43}},
			},
			{
				Id:        3,
				MappingId: 1,
				Address:   19267106,
				Line:      []*profilev1.Line{{FunctionId: 3, Line: 40}},
			},
			{
				Id:        4,
				MappingId: 1,
				Address:   19499013,
				Line:      []*profilev1.Line{{FunctionId: 4, Line: 42}},
			},
		},
		Function: []*profilev1.Function{
			{
				Id:         1,
				Name:       13,
				SystemName: 13,
				Filename:   14,
			},
			{
				Id:         2,
				Name:       15,
				SystemName: 15,
				Filename:   14,
			},
			{
				Id:         3,
				Name:       16,
				SystemName: 16,
				Filename:   17,
			},
			{
				Id:         4,
				Name:       18,
				SystemName: 18,
				Filename:   14,
			},
		},
		StringTable:   stringTable,
		TimeNanos:     1654798932062349000,
		DurationNanos: 20246727106,
		PeriodType: &profilev1.ValueType{
			Type: 3,
			Unit: 4,
		},
		Period: 10000000,
	}

	var m ProfileMerge
	require.NoError(t, m.Merge(a, true))
	require.NoError(t, m.Merge(b, true))

	testhelper.EqualProto(t, expected, m.Profile())
}

func TestMergeEmpty(t *testing.T) {
	var m ProfileMerge

	err := m.Merge(&profilev1.Profile{
		SampleType: []*profilev1.ValueType{
			{
				Type: 2,
				Unit: 1,
			},
		},
		PeriodType: &profilev1.ValueType{
			Type: 2,
			Unit: 1,
		},
		StringTable: []string{"", "nanoseconds", "cpu"},
	}, true)
	require.NoError(t, err)
	err = m.Merge(&profilev1.Profile{
		Sample: []*profilev1.Sample{
			{
				LocationId: []uint64{1},
				Value:      []int64{1},
			},
		},
		Location: []*profilev1.Location{
			{
				Id:        1,
				MappingId: 1,
				Line:      []*profilev1.Line{{FunctionId: 1, Line: 1}},
			},
		},
		Function: []*profilev1.Function{
			{
				Id:   1,
				Name: 1,
			},
		},
		SampleType: []*profilev1.ValueType{
			{
				Type: 3,
				Unit: 2,
			},
		},
		PeriodType: &profilev1.ValueType{
			Type: 3,
			Unit: 2,
		},
		Mapping: []*profilev1.Mapping{
			{
				Id: 1,
			},
		},
		StringTable: []string{"", "bar", "nanoseconds", "cpu"},
	}, true)
	require.NoError(t, err)
}

// Benchmark_Merge_self/pprof.Merge-10                	    2722	    421419 ns/op
// Benchmark_Merge_self/profile.Merge-10              	     802	   1417907 ns/op
func Benchmark_Merge_self(b *testing.B) {
	d, err := os.ReadFile("testdata/go.cpu.labels.pprof")
	require.NoError(b, err)

	b.Run("pprof.Merge", func(b *testing.B) {
		p, err := RawFromBytes(d)
		require.NoError(b, err)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var m ProfileMerge
			require.NoError(b, m.Merge(p.CloneVT(), true))
		}
	})

	b.Run("profile.Merge", func(b *testing.B) {
		p, err := profile.ParseData(d)
		require.NoError(b, err)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err = profile.Merge([]*profile.Profile{p.Copy()})
			require.NoError(b, err)
		}
	})
}
