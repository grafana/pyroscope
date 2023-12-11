// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

//go:build !windows

package fastdelta

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"github.com/google/pprof/profile"
	"github.com/klauspost/compress/gzip"
	"github.com/richardartoul/molecule"
	"github.com/richardartoul/molecule/src/protowire"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	heapFile    = "heap.pprof"
	bigHeapFile = "big-heap.pprof"
)

// retain prevents GC-collection of the data structures used during
// benchmarking. This is allows us to report heap-inuse-B/op and to take
// useful -memprofile=mem.pprof profiles.
var retain struct {
	DC   *DeltaComputer
	Prev *profile.Profile
}

var implementations = []struct {
	Name string
	Func func() func([]byte, io.Writer) error
}{
	{
		Name: "fastdelta",
		Func: func() func([]byte, io.Writer) error {
			dc := NewDeltaComputer(
				vt("alloc_objects", "count"),
				vt("alloc_space", "bytes"),
			)
			retain.DC = dc
			return func(prof []byte, w io.Writer) error {
				return dc.Delta(prof, w)
			}
		},
	},
	{
		Name: "pprof",
		Func: func() func([]byte, io.Writer) error {
			var prev *profile.Profile
			return func(b []byte, w io.Writer) error {
				prof, err := profile.ParseData(b)
				if err != nil {
					return err
				}
				delta := prof
				if prev != nil {
					if err := prev.ScaleN([]float64{-1, -1, 0, 0}); err != nil {
						return err
					} else if delta, err = profile.Merge([]*profile.Profile{prev, prof}); err != nil {
						return err
					} else if err := delta.WriteUncompressed(w); err != nil {
						return err
					}
				} else if _, err := w.Write(b); err != nil {
					return err
				}
				prev = prof
				retain.Prev = prev
				return nil
			}
		},
	},
}

// dc is a package var so we can look at the heap profile after benchmarking to
// understand heap in-use.
// IMPORTANT: Use with -memprofilerate=1 to get useful values.
var dc *DeltaComputer

func BenchmarkDelta(b *testing.B) {
	for _, impl := range implementations {
		b.Run(impl.Name, func(b *testing.B) {
			for _, f := range []string{heapFile, bigHeapFile} {
				testFile := filepath.Join("testdata", f)
				b.Run(f, func(b *testing.B) {
					before, err := os.ReadFile(testFile)
					if err != nil {
						b.Fatal(err)
					}
					after, err := os.ReadFile(testFile)
					if err != nil {
						b.Fatal(err)
					}

					b.Run("setup", func(b *testing.B) {
						b.SetBytes(int64(len(before)))
						b.ReportAllocs()

						for i := 0; i < b.N; i++ {
							deltaFn := impl.Func()
							if err := deltaFn(before, io.Discard); err != nil {
								b.Fatal(err)
							} else if err := deltaFn(after, io.Discard); err != nil {
								b.Fatal(err)
							}
						}
					})

					b.Run("steady-state", func(b *testing.B) {
						b.SetBytes(int64(len(before)))
						b.ReportAllocs()

						deltaFn := impl.Func()
						if err := deltaFn(before, io.Discard); err != nil {
							b.Fatal(err)
						}

						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							if err := deltaFn(after, ioutil.Discard); err != nil {
								b.Fatal(err)
							}
						}
						b.StopTimer()
						reportHeapUsage(b)
					})
				})
			}
		})
	}
}

// reportHeapUsage reports how much heap memory is used by the fastdelta
// implementation.
// IMPORTANT: Use with -memprofilerate=1 to get useful values.
func reportHeapUsage(b *testing.B) {
	// force GC often enough so that our heap profile is up-to-date.
	// TODO(fg) not sure if this needs to be 2 or 3 times ...
	runtime.GC()
	runtime.GC()
	runtime.GC()

	var buf bytes.Buffer
	pprof.Lookup("heap").WriteTo(&buf, 0)
	profile, err := profile.Parse(&buf)
	require.NoError(b, err)

	var sum float64
nextSample:
	for _, s := range profile.Sample {
		if s.Value[3] == 0 {
			continue
		}
		for _, loc := range s.Location {
			for _, line := range loc.Line {
				if strings.Contains(line.Function.Name, "profiler/internal/fastdelta.(*DeltaComputer)") ||
					strings.Contains(line.Function.Name, "github.com/google/pprof") {
					sum += float64(s.Value[3])
					continue nextSample
				}
			}
		}
	}

	b.ReportMetric(sum, "heap-inuse-B/op")
}

func BenchmarkMakeGolden(b *testing.B) {
	for _, f := range []string{heapFile, bigHeapFile} {
		testFile := "testdata/" + f
		b.Run(testFile, func(b *testing.B) {
			b.ReportAllocs()
			before, err := os.ReadFile(testFile)
			if err != nil {
				b.Fatal(err)
			}
			after, err := os.ReadFile(testFile)
			if err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				psink = makeGolden(b, before, after,
					vt("alloc_objects", "count"), vt("alloc_space", "bytes"))
			}
		})
	}
}

var (
	sink  []byte
	psink *profile.Profile
)

func TestFastDeltaComputer(t *testing.T) {
	tests := []struct {
		Name     string
		Before   string
		After    string
		Duration int64
		Fields   []ValueType
	}{
		{
			Name:     "heap",
			Before:   "testdata/heap.before.pprof",
			After:    "testdata/heap.after.pprof",
			Duration: 5960465000,
			Fields: []ValueType{
				vt("alloc_objects", "count"),
				vt("alloc_space", "bytes"),
			},
		},
		{
			Name:     "block",
			Before:   "testdata/block.before.pprof",
			After:    "testdata/block.after.pprof",
			Duration: 1144928000,
			Fields: []ValueType{
				vt("contentions", "count"),
				vt("delay", "nanoseconds"),
			},
		},
		// The following tests were generated through
		// TestRepeatedHeapProfile failures.
		{
			Name:   "heap stress",
			Before: "testdata/stress-failure.before.pprof",
			After:  "testdata/stress-failure.after.pprof",
			Fields: []ValueType{
				vt("alloc_objects", "count"),
				vt("alloc_space", "bytes"),
			},
		},
		{
			Name:   "heap stress 2",
			Before: "testdata/stress-failure.2.before.pprof",
			After:  "testdata/stress-failure.2.after.pprof",
			Fields: []ValueType{
				vt("alloc_objects", "count"),
				vt("alloc_space", "bytes"),
			},
		},
		{
			Name:   "heap stress 3",
			Before: "testdata/stress-failure.3.before.pprof",
			After:  "testdata/stress-failure.3.after.pprof",
			Fields: []ValueType{
				vt("alloc_objects", "count"),
				vt("alloc_space", "bytes"),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			before, err := os.ReadFile(tc.Before)
			if err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(tc.After)
			if err != nil {
				t.Fatal(err)
			}

			dc := NewDeltaComputer(tc.Fields...)
			if err := dc.Delta(before, io.Discard); err != nil {
				t.Fatal(err)
			}
			// TODO: check the output of the first Delta. Should be unchanged

			data := new(bytes.Buffer)
			if err := dc.Delta(after, data); err != nil {
				t.Fatal(err)
			}

			delta, err := profile.ParseData(data.Bytes())
			if err != nil {
				t.Fatalf("parsing delta profile: %s", err)
			}

			golden := makeGolden(t, before, after, tc.Fields...)

			golden.Scale(-1)
			diff, err := profile.Merge([]*profile.Profile{delta, golden})
			if err != nil {
				t.Fatal(err)
			}
			if len(diff.Sample) != 0 {
				t.Errorf("non-empty diff from golden vs delta: %v", diff)
				t.Errorf("got: %v", delta)
				t.Errorf("want: %v", golden)
			}

			if tc.Duration != 0 {
				require.Equal(t, tc.Duration, delta.DurationNanos)
			}
		})
	}
}

func makeGolden(t testing.TB, before, after []byte, fields ...ValueType) *profile.Profile {
	t.Helper()
	b, err := profile.ParseData(before)
	if err != nil {
		t.Fatal(err)
	}
	a, err := profile.ParseData(after)
	if err != nil {
		t.Fatal(err)
	}

	ratios := make([]float64, len(b.SampleType))
	for i, v := range b.SampleType {
		for _, f := range fields {
			if f.Type == v.Type {
				ratios[i] = -1
			}
		}
	}
	if err := b.ScaleN(ratios); err != nil {
		t.Fatal(err)
	}

	c, err := profile.Merge([]*profile.Profile{b, a})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestDurationAndTime(t *testing.T) {
	// given
	dc := NewDeltaComputer(
		vt("alloc_objects", "count"),
		vt("alloc_space", "bytes"),
	)
	heapBytes, err := os.ReadFile("testdata/big-heap.pprof")
	require.NoError(t, err)
	inputPprof, err := profile.ParseData(heapBytes)
	require.NoError(t, err)

	// The first expected duration is the same as the first pprof fed to dc.
	// We need to invoke dc.Delta at least 3 times to exercise the duration logic.
	fixtures := []int64{inputPprof.DurationNanos, 0, 0, 0}
	for i := 1; i < len(fixtures); i++ {
		fixtures[i] = int64(i) * 10
	}

	inputBuf := new(bytes.Buffer)
	outputBuf := new(bytes.Buffer)
	for i := 1; i < len(fixtures); i++ {
		inputBuf.Reset()
		outputBuf.Reset()
		require.NoError(t, inputPprof.WriteUncompressed(inputBuf))
		err = dc.Delta(inputBuf.Bytes(), outputBuf)
		deltaPprof, err := profile.ParseData(outputBuf.Bytes())
		require.NoError(t, err)

		expectedDuration := fixtures[i-1]
		require.Equal(t, expectedDuration, deltaPprof.DurationNanos)
		require.Equal(t, inputPprof.TimeNanos, deltaPprof.TimeNanos)

		// advance the time
		inputPprof.TimeNanos += fixtures[i]
	}
}

func TestCompaction(t *testing.T) {
	// given

	bigHeapBytes, err := os.ReadFile("testdata/big-heap.pprof")
	require.NoError(t, err)
	zeroDeltaPprof, err := profile.ParseData(bigHeapBytes)
	require.NoError(t, err)
	// add some string values
	zeroDeltaPprof.Comments = []string{"hello", "world"}
	zeroDeltaPprof.DefaultSampleType = "inuse_objects"
	zeroDeltaPprof.DropFrames = "drop 'em"
	zeroDeltaPprof.KeepFrames = "keep 'em"

	zeroDeltaBuf := &bytes.Buffer{}
	require.NoError(t, zeroDeltaPprof.WriteUncompressed(zeroDeltaBuf))

	dc := NewDeltaComputer(
		vt("alloc_objects", "count"),
		vt("alloc_space", "bytes"),
	)
	buf := new(bytes.Buffer)
	err = dc.Delta(zeroDeltaBuf.Bytes(), buf)
	zeroDeltaBytes := buf.Bytes()
	require.NoError(t, err)
	require.Equal(t, zeroDeltaBuf.Len(), len(zeroDeltaBytes))

	// when

	// create a value delta
	require.NoError(t, err)
	for _, s := range zeroDeltaPprof.Sample {
		s.Value[2] = 0
		s.Value[3] = 0
	}
	zeroDeltaPprof.Sample[0].Value[0] += 42
	bufNext := &bytes.Buffer{}
	require.NoError(t, zeroDeltaPprof.WriteUncompressed(bufNext))
	buf.Reset()
	err = dc.Delta(bufNext.Bytes(), buf)
	delta := buf.Bytes()
	require.NoError(t, err)
	firstDeltaPprof, err := profile.ParseData(delta)
	require.NoError(t, err)

	// then

	require.Len(t, firstDeltaPprof.Sample, 1, "Only one expected sample")
	require.Len(t, firstDeltaPprof.Mapping, 1, "Only one expected mapping")
	require.Len(t, firstDeltaPprof.Location, 3, "Location should be GCd")
	require.Len(t, firstDeltaPprof.Function, 3, "Function should be GCd")
	require.Equal(t, int64(42), firstDeltaPprof.Sample[0].Value[0])

	// make sure we shrunk the string table too (85K+ without pruning)
	// note that most of the delta buffer is full of empty strings, highly compressible
	require.Less(t, len(delta), 3720)

	// string table checks on Profile message string fields
	require.Equal(t, []string{"hello", "world"}, firstDeltaPprof.Comments)
	require.Equal(t, "inuse_objects", firstDeltaPprof.DefaultSampleType)
	require.Equal(t, "drop 'em", firstDeltaPprof.DropFrames)
	require.Equal(t, "keep 'em", firstDeltaPprof.KeepFrames)

	// check a mapping
	m := firstDeltaPprof.Mapping[0]
	require.Equal(t, "537aaf6df5ba3cc343a7c78738e4fe3890ab9782", m.BuildID)
	require.Equal(t, "/usr/local/bin/nicky", m.File)

	// check a value type
	vt := firstDeltaPprof.SampleType[0]
	require.Equal(t, "alloc_objects", vt.Type)
	require.Equal(t, "count", vt.Unit)

	// check a function
	f := firstDeltaPprof.Sample[0].Location[0].Line[0].Function
	require.Equal(t, "hawaii-alabama-artist", f.SystemName)
	require.Equal(t, "hawaii-alabama-artist", f.Name)
	require.Equal(t, "/wisconsin/video/beer/spring/delta/pennsylvania/four", f.Filename)

	// check a label
	l := firstDeltaPprof.Sample[0].NumLabel
	require.Contains(t, l, "bytes")
}

func TestSampleHashingConsistency(t *testing.T) {
	// f builds a profile with a single sample which has labels in the given
	// order. We build the profile ourselves because we can control the
	// precise binary encoding of the profile.
	f := func(labels ...string) []byte {
		var err error
		b := new(bytes.Buffer)
		ps := molecule.NewProtoStream(b)
		err = ps.Embedded(1, func(ps *molecule.ProtoStream) error {
			// sample_type
			err = ps.Int64(1, 1) // type
			require.NoError(t, err)
			err = ps.Int64(2, 2) // unit
			require.NoError(t, err)
			return nil
		})
		require.NoError(t, err)
		err = ps.Embedded(11, func(ps *molecule.ProtoStream) error {
			// period_type
			err = ps.Int64(1, 1) // type
			require.NoError(t, err)
			err = ps.Int64(2, 2) // unit
			require.NoError(t, err)
			return nil
		})
		require.NoError(t, err)
		err = ps.Int64(12, 1) // period
		require.NoError(t, err)
		err = ps.Int64(9, 1) // time_nanos
		require.NoError(t, err)
		err = ps.Embedded(4, func(ps *molecule.ProtoStream) error {
			// location
			err = ps.Uint64(1, 1) // location ID
			require.NoError(t, err)
			err = ps.Uint64(2, 1) // mapping ID
			require.NoError(t, err)
			err = ps.Uint64(3, 0x42) // address
			require.NoError(t, err)
			return nil
		})
		require.NoError(t, err)
		err = ps.Embedded(2, func(ps *molecule.ProtoStream) error {
			// samples
			err = ps.Uint64(1, 1) // location ID
			require.NoError(t, err)
			err = ps.Uint64(2, 1) // value
			require.NoError(t, err)
			for i := 0; i < len(labels); i += 2 {
				err = ps.Embedded(3, func(ps *molecule.ProtoStream) error {
					err = ps.Uint64(1, uint64(i)+3) // key strtab offset
					require.NoError(t, err)
					err = ps.Uint64(2, uint64(i)+4) // str strtab offset
					require.NoError(t, err)
					return nil
				})
				require.NoError(t, err)
			}
			return nil
		})
		require.NoError(t, err)
		err = ps.Embedded(3, func(ps *molecule.ProtoStream) error {
			// mapping
			err = ps.Uint64(1, 1) // ID
			require.NoError(t, err)
			return nil
		})
		require.NoError(t, err)
		// don't need functions
		buf := b.Bytes()
		writeString := func(s string) {
			buf = protowire.AppendVarint(buf, (6<<3)|2)
			buf = protowire.AppendVarint(buf, uint64(len(s)))
			buf = append(buf, s...)
		}
		writeString("")     // 0 -- molecule doesn't let you write 0-length with ProtoStream
		writeString("type") // 1
		writeString("unit") // 2
		for i := 0; i < len(labels); i += 2 {
			writeString(labels[i])
			writeString(labels[i+1])
		}
		return buf
	}
	a := f("foo", "bar", "abc", "123")
	b := f("abc", "123", "foo", "bar")

	// double-checks that our generated profiles are valid
	require.NotEqual(t, a, b)
	_, err := profile.ParseData(a)
	require.NoError(t, err)
	_, err = profile.ParseData(b)
	require.NoError(t, err)

	dc := NewDeltaComputer(vt("type", "unit"))
	err = dc.Delta(a, io.Discard)
	require.NoError(t, err)
	buf := new(bytes.Buffer)
	err = dc.Delta(b, buf)
	require.NoError(t, err)

	p, err := profile.ParseData(buf.Bytes())
	require.NoError(t, err)
	// There should be no samples because we didn't actually change the
	// profile, just the order of the labels.
	require.Empty(t, p.Sample)
}

func vt(vtype, vunit string) ValueType {
	return ValueType{Type: vtype, Unit: vunit}
}

type badWriter struct{}

func (badWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("fail")
}

func TestRecovery(t *testing.T) {
	before, err := os.ReadFile("testdata/heap.before.pprof")
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile("testdata/heap.after.pprof")
	if err != nil {
		t.Fatal(err)
	}

	fields := []ValueType{
		vt("alloc_objects", "count"),
		vt("alloc_space", "bytes"),
	}

	dc := NewDeltaComputer(fields...)
	if err := dc.Delta(before, badWriter{}); err == nil {
		t.Fatal("delta out to bad writer spuriously succeeded")
	}

	// dc is now in a bad state, and needs to recover. The next write should
	// accept the input to re-set its state, but shouldn't claim to have
	// successfully computed a delta profile
	if err := dc.Delta(before, io.Discard); err == nil {
		t.Fatal("delta after bad state spuriously succeeded")
	}

	data := new(bytes.Buffer)
	if err := dc.Delta(after, data); err != nil {
		t.Fatal(err)
	}

	delta, err := profile.ParseData(data.Bytes())
	if err != nil {
		t.Fatalf("parsing delta profile: %s", err)
	}

	golden := makeGolden(t, before, after, fields...)

	golden.Scale(-1)
	diff, err := profile.Merge([]*profile.Profile{delta, golden})
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Sample) != 0 {
		t.Errorf("non-empty diff from golden vs delta: %v", diff)
		t.Errorf("got: %v", delta)
		t.Errorf("want: %v", golden)
	}
}

//go:noinline
func makeGarbage() {
	x := make([]int, rand.Intn(10000)+1)
	b, _ := json.Marshal(x)
	json.NewDecoder(bytes.NewReader(b)).Decode(&x)
	// Force GC so that we clean up the allocations and they show up
	// in the profile.
	runtime.GC()
}

// left & right are recursive functions which call one another randomly,
// and eventually call makeGarbage. We get 2^N possible combinations of
// left and right in the stacks for a depth-N recursion. This lets us
// artificially inflate the size of the profile. This is inspired by seeing
// something similar in a profile where a program did a lot of sorting.

//go:noinline
func left(n int) {
	if n <= 0 {
		makeGarbage()
		return
	}
	if rand.Intn(2) == 0 {
		left(n - 1)
	} else {
		right(n - 1)
	}
}

//go:noinline
func right(n int) {
	if n <= 0 {
		makeGarbage()
		return
	}
	if rand.Intn(2) == 0 {
		left(n - 1)
	} else {
		right(n - 1)
	}
}

func TestRepeatedHeapProfile(t *testing.T) {
	if os.Getenv("DELTA_PROFILE_HEAP_STRESS_TEST") == "" {
		t.Skip("This test is resource-intensive. To run it, set the DELTA_PROFILE_HEAP_STRESS_TEST environment variable")
	}
	readProfile := func(name string) []byte {
		b := new(bytes.Buffer)
		if err := pprof.Lookup(name).WriteTo(b, 0); err != nil {
			t.Fatal(err)
		}
		r, _ := gzip.NewReader(b)
		p, _ := io.ReadAll(r)
		return p
	}

	fields := []ValueType{
		vt("alloc_objects", "count"),
		vt("alloc_space", "bytes"),
	}

	dc := NewDeltaComputer(fields...)

	before := readProfile("heap")
	if err := dc.Delta(before, io.Discard); err != nil {
		t.Fatal(err)
	}

	iters := 100
	if testing.Short() {
		iters = 10
	}
	for i := 0; i < iters; i++ {
		// Create a bunch of new allocations so there's something to diff.
		for j := 0; j < 200; j++ {
			left(10)
		}
		after := readProfile("heap")

		data := new(bytes.Buffer)
		if err := dc.Delta(after, data); err != nil {
			t.Fatal(err)
		}
		delta, err := profile.ParseData(data.Bytes())
		if err != nil {
			t.Fatalf("parsing delta profile: %s", err)
		}

		golden := makeGolden(t, before, after, fields...)

		golden.Scale(-1)
		diff, err := profile.Merge([]*profile.Profile{delta, golden})
		if err != nil {
			t.Fatal(err)
		}
		if len(diff.Sample) != 0 {
			t.Errorf("non-empty diff from golden vs delta: %v", diff)
			t.Errorf("got: %v", delta)
			t.Errorf("want: %v", golden)
			now := time.Now().Format(time.RFC3339)
			os.WriteFile(fmt.Sprintf("failure-before-%s", now), before, 0o660)
			os.WriteFile(fmt.Sprintf("failure-after-%s", now), after, 0o660)
		}
		before = after
	}
}

func TestDuplicateSample(t *testing.T) {
	f := func(labels ...string) []byte {
		var err error
		b := new(bytes.Buffer)
		ps := molecule.NewProtoStream(b)
		err = ps.Embedded(1, func(ps *molecule.ProtoStream) error {
			// sample_type
			err = ps.Int64(1, 1) // type
			require.NoError(t, err)
			err = ps.Int64(2, 2) // unit
			require.NoError(t, err)
			return nil
		})
		require.NoError(t, err)
		err = ps.Embedded(11, func(ps *molecule.ProtoStream) error {
			// period_type
			err = ps.Int64(1, 1) // type
			require.NoError(t, err)
			err = ps.Int64(2, 2) // unit
			require.NoError(t, err)
			return nil
		})
		require.NoError(t, err)
		err = ps.Int64(12, 1) // period
		require.NoError(t, err)
		err = ps.Int64(9, 1) // time_nanos
		require.NoError(t, err)
		err = ps.Embedded(4, func(ps *molecule.ProtoStream) error {
			// location
			err = ps.Uint64(1, 1) // location ID
			require.NoError(t, err)
			err = ps.Uint64(2, 1) // mapping ID
			require.NoError(t, err)
			err = ps.Uint64(3, 0x42) // address
			require.NoError(t, err)
			return nil
		})
		require.NoError(t, err)
		err = ps.Embedded(2, func(ps *molecule.ProtoStream) error {
			// samples
			err = ps.Uint64(1, 1) // location ID
			require.NoError(t, err)
			err = ps.Uint64(2, 1) // value
			require.NoError(t, err)
			for i := 0; i < len(labels); i += 2 {
				err = ps.Embedded(3, func(ps *molecule.ProtoStream) error {
					err = ps.Uint64(1, uint64(i)+3) // key strtab offset
					require.NoError(t, err)
					err = ps.Uint64(2, uint64(i)+4) // str strtab offset
					require.NoError(t, err)
					return nil
				})
				require.NoError(t, err)
			}
			return nil
		})
		require.NoError(t, err)
		err = ps.Embedded(2, func(ps *molecule.ProtoStream) error {
			// samples
			err = ps.Uint64(1, 1) // location ID
			require.NoError(t, err)
			err = ps.Uint64(2, 1) // value
			require.NoError(t, err)
			for i := 0; i < len(labels); i += 2 {
				err = ps.Embedded(3, func(ps *molecule.ProtoStream) error {
					err = ps.Uint64(1, uint64(i)+3) // key strtab offset
					require.NoError(t, err)
					err = ps.Uint64(2, uint64(i)+4) // str strtab offset
					require.NoError(t, err)
					return nil
				})
				require.NoError(t, err)
			}
			return nil
		})
		require.NoError(t, err)
		err = ps.Embedded(3, func(ps *molecule.ProtoStream) error {
			// mapping
			err = ps.Uint64(1, 1) // ID
			require.NoError(t, err)
			return nil
		})
		require.NoError(t, err)
		// don't need functions
		buf := b.Bytes()
		writeString := func(s string) {
			buf = protowire.AppendVarint(buf, (6<<3)|2)
			buf = protowire.AppendVarint(buf, uint64(len(s)))
			buf = append(buf, s...)
		}
		writeString("")     // 0 -- molecule doesn't let you write 0-length with ProtoStream
		writeString("type") // 1
		writeString("unit") // 2
		for i := 0; i < len(labels); i += 2 {
			writeString(labels[i])
			writeString(labels[i+1])
		}
		return buf
	}
	a := f("foo", "bar", "abc", "123")

	// double-checks that our generated profiles are valid
	_, err := profile.ParseData(a)
	require.NoError(t, err)

	dc := NewDeltaComputer(vt("type", "unit"))

	err = dc.Delta(a, io.Discard)
	require.NoError(t, err)
	for i := 0; i < 10; i++ {
		buf := new(bytes.Buffer)
		err = dc.Delta(a, buf)
		require.NoError(t, err)

		p, err := profile.ParseData(buf.Bytes())
		require.NoError(t, err)
		t.Logf("%v", p)
		// There should be no samples because we didn't actually change the
		// profile, just the order of the labels.
		assert.Empty(t, p.Sample)
	}
}
