package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"runtime"
	pprof2 "runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	_ "unsafe"

	"github.com/grafana/pyroscope-go/godeltaprof/testutil"
	"github.com/grafana/pyroscope/pkg/fastdelta"
	"github.com/grafana/pyroscope/pkg/og/convert/pprof/bench"
	"github.com/grafana/pyroscope/pkg/pprof"
)

const tmpDir = "/tmp/godeltaprofbench/"

func init() {
	http.HandleFunc("/debug/pprof/MemProfile/", MemProfile)
	http.HandleFunc("/debug/pprof/MemProfileRunTest/", runTestsAndBenchmarksOverHTTP)
}

var benchRunNumber atomic.Int32
var cnt atomic.Int32

func init() {
	if err := os.RemoveAll(tmpDir); err != nil {
		panic(err)
	}
	if err := os.MkdirAll(tmpDir, 0777); err != nil {
		panic(err)
	}
	testing.Init()

	testing.SetBenchTimeDuration(10 * time.Second)

}

func MemProfile(writer http.ResponseWriter, request *http.Request) {
	runtime.GC()
	var p []runtime.MemProfileRecord
	n, ok := runtime.MemProfile(nil, true)
	for {
		p = make([]runtime.MemProfileRecord, n+50)
		n, ok = runtime.MemProfile(p, true)
		if ok {
			p = p[0:n]
			break
		}
	}
	json, err := json.Marshal(p)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte(err.Error()))
	} else {
		writer.WriteHeader(http.StatusOK)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(json)

		dumpname := fmt.Sprintf(tmpDir+"memprofile-%d.json", cnt.Add(1))
		_ = os.WriteFile(dumpname, json, 0666)
	}
}

func runTestsAndBenchmarksOverHTTP(writer http.ResponseWriter, request *http.Request) {
	files := request.URL.Query()["files"]
	runTestsAndBenchmarks(writer, files)
}

var mutex sync.Mutex

func runTestsAndBenchmarks(writer io.Writer, files []string) {
	mutex.Lock()
	defer mutex.Unlock()
	defer func() {
		if r := recover(); r != nil {
			writer.Write([]byte("recovered from panic\n"))
			writer.Write([]byte(fmt.Sprintf("%+v\n", r)))
		}
	}()

	unmarshalTestData(files)

	runTestComparison(writer) // this is failing because of bugs in golang runtime related to generics

	no := benchRunNumber.Add(1)
	runOneBench(writer, no, "BenchmarkOG", BenchmarkOG)
	runOneBench(writer, no, "BenchmarkGodeltaprof", BenchmarkGodeltaprof)
	runOneBench(writer, no, "BenchmarkFastDelta", BenchmarkFastDelta)
}

func runOneBench(w io.Writer, no int32, name string, f func(b *testing.B)) testing.BenchmarkResult {
	profBuf := bytes.NewBuffer(make([]byte, 0, 1024*1024))
	err := pprof2.StartCPUProfile(profBuf)
	if err != nil {
		panic(fmt.Errorf("failed to start cpu profiling %w", err))
	}
	defer func() {
		pprof2.StopCPUProfile()
		_ = os.WriteFile(tmpDir+name+fmt.Sprintf("%d.pb.gz", no), profBuf.Bytes(), 0666)
	}()

	w.Write([]byte(name + "\n"))
	res := testing.Benchmark(f)
	w.Write([]byte(fmt.Sprintf("%+v\n", res)))
	w.Write([]byte(fmt.Sprintf("profile sizes: %+v\n", profileSizes)))
	return res
}

var testdata [][]runtime.MemProfileRecord
var profileSizes []int

func unmarshalTestData(files []string) {
	profileSizes = make([]int, len(files))
	testdata = make([][]runtime.MemProfileRecord, 0, len(files))
	for _, f := range files {
		var it []runtime.MemProfileRecord
		bs, err := os.ReadFile(f)
		if err != nil {
			panic(err)
		}
		err = json.Unmarshal(bs, &it)
		if err != nil {
			panic(err)
		}
		testdata = append(testdata, it)
	}
}

func BenchmarkOG(b *testing.B) {
	buf := bytes.NewBuffer(make([]byte, 0, 1024*1024))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j, td := range testdata {
			buf.Reset()
			err := writeHeapProto(buf, td, int64(runtime.MemProfileRate), "alloc_space")
			if err != nil {
				b.Fatal(err)
			}
			profileSizes[j] = buf.Len()
		}
	}
}

func BenchmarkGodeltaprof(b *testing.B) {
	buf := bytes.NewBuffer(make([]byte, 0, 1024*1024))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := testutil.NewHeapProfiler(true, true)
		for j, td := range testdata {
			buf.Reset()
			err := p.WriteHeapProto(buf, td, int64(runtime.MemProfileRate), "alloc_space")
			if err != nil {
				b.Fatal(err)
			}
			profileSizes[j] = buf.Len()
		}
	}
}

func BenchmarkFastDelta(b *testing.B) {
	buf := bytes.NewBuffer(make([]byte, 0, 1024*1024))

	for i := 0; i < b.N; i++ {
		d := fastdelta.NewDeltaComputer([]fastdelta.ValueType{
			{Type: "alloc_objects", Unit: "count"},
			{Type: "alloc_space", Unit: "bytes"},
		}...)
		for j, td := range testdata {
			buf.Reset()
			err := writeHeapProto(buf, td, int64(runtime.MemProfileRate), "alloc_space")
			if err != nil {
				b.Fatal(err)
			}
			ogProfile := buf.Bytes()
			fastdeltaProfile, err := computeDelta(d, ogProfile)

			if err != nil {
				fmt.Println(err.Error())
				b.Fatal(err)
			}
			_ = fastdeltaProfile
			profileSizes[j] = len(fastdeltaProfile)
		}
	}
}

//go:linkname writeHeapProto runtime/pprof.writeHeapProto
func writeHeapProto(w io.Writer, p []runtime.MemProfileRecord, rate int64, defaultSampleType string) error

func runTestComparison(w io.Writer) {
	dh := testutil.NewHeapProfiler(true, true)
	d := fastdelta.NewDeltaComputer([]fastdelta.ValueType{
		{Type: "alloc_objects", Unit: "count"},
		{Type: "alloc_space", Unit: "bytes"},
	}...)
	failed := false
	for itd, testdatum := range testdata {
		buf := bytes.NewBuffer(nil)
		err := writeHeapProto(buf, testdatum, int64(runtime.MemProfileRate), "alloc_space")
		noError(err)
		ogProfile := buf.Bytes()

		buf = bytes.NewBuffer(nil)
		err = dh.WriteHeapProto(buf, testdatum, int64(runtime.MemProfileRate), "alloc_space")
		noError(err)
		godeltaprofProfile := buf.Bytes()

		fastdeltaProfile, err := computeDelta(d, ogProfile)
		noError(err)

		ogProfileParsed, err := pprof.RawFromBytes(ogProfile)
		noError(err)

		godeltaprofProfileParsed, err := pprof.RawFromBytes(godeltaprofProfile)
		noError(err)

		fastdeltaProfileParsed, err := pprof.RawFromBytes(fastdeltaProfile)
		noError(err)

		for i := 0; i < 4; i++ {
			godeltaprofCollapsed := bench.StackCollapseProto(godeltaprofProfileParsed.Profile, i, 1)
			fastdeltaCollapsed := bench.StackCollapseProto(fastdeltaProfileParsed.Profile, i, 1)
			ogCollapsed := bench.StackCollapseProto(ogProfileParsed.Profile, i, 1)
			os.WriteFile(fmt.Sprintf(tmpDir+"og-%d-%d.txt", itd, i), []byte(strings.Join(ogCollapsed, "\n")), 0666)
			os.WriteFile(fmt.Sprintf(tmpDir+"godeltaprof-%d-%d.txt", itd, i), []byte(strings.Join(godeltaprofCollapsed, "\n")), 0666)
			os.WriteFile(fmt.Sprintf(tmpDir+"fastdelta-%d-%d.txt", itd, i), []byte(strings.Join(fastdeltaCollapsed, "\n")), 0666)
			if !reflect.DeepEqual(godeltaprofCollapsed, fastdeltaCollapsed) {
				failed = true
				w.Write([]byte(fmt.Sprintf("profile mismatch t itd %d i %d\n", itd, i)))
			}
		}
	}
	if failed {
		w.Write([]byte("FAIL\n"))
	} else {
		w.Write([]byte("PASS\n"))
	}
}

var re = regexp.MustCompile("\\[[^]*]")

func stripGenerics(s string) string {
	return re.ReplaceAllString(s, "[...]")
}

func noError(err error) {
	if err != nil {
		panic(err)
	}
}
