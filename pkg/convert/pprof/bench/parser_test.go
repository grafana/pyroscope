package bench

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"github.com/pyroscope-io/pyroscope/pkg/convert/pprof"
	"github.com/pyroscope-io/pyroscope/pkg/convert/pprof/streaming"
	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/util/form"
	"io"
	"io/fs"
	"mime/multipart"
	"os"
	"sort"
	"strings"

	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"io/ioutil"
	"net/http"

	"testing"
	"time"
)

const benchmarkCorps = "../../../../../pprof-testdata"
const compareCorpus = "../../../../../pprof-testdata"

const pprofSmall = benchmarkCorps +
	"/2022-10-08T00:44:10Z-55903298-d296-4730-a28d-9dcc7c5e25d6.txt"
const pprofBig = benchmarkCorps +
	"/2022-10-08T00:07:00Z-911c824f-a086-430c-99d7-315a53b58095.txt"

// GOEXPERIMENT=arenas go test -v -test.count=10 -test.run=none -bench=".*Streaming.*"  ./pkg/convert/pprof/bench
var putter = &MockPutter{}

const benchWithoutGzip = true
const benchmarkCorpusSize = 5

func TestCompare(t *testing.T) {
	corpus := readCorpus(compareCorpus, benchWithoutGzip)
	if len(corpus) == 0 {
		t.Skip("empty corpus")
		return
	}
	for _, testType := range streamingTestTypes {
		t.Run(fmt.Sprintf("TestCompare_pool_%v_arenas_%v", testType.pool, testType.arenas), func(t *testing.T) {
			for _, c := range corpus {
				testCompareOne(t, c, testType)
			}
		})
	}
}

func BenchmarkSmallStreaming(b *testing.B) {
	t := readCorpusItemFile(pprofSmall, benchWithoutGzip)
	for _, testType := range streamingTestTypes {
		b.Run(fmt.Sprintf("BenchmarkSmallStreaming_pool_%v_arenas_%v", testType.pool, testType.arenas), func(b *testing.B) {
			benchmarkStreamingOne(b, t, testType)
		})
	}
}

func BenchmarkBigStreaming(b *testing.B) {
	t := readCorpusItemFile(pprofBig, benchWithoutGzip)
	for _, testType := range streamingTestTypes {
		b.Run(fmt.Sprintf("BenchmarkBigStreaming_pool_%v_arenas_%v", testType.pool, testType.arenas), func(b *testing.B) {
			benchmarkStreamingOne(b, t, testType)
		})
	}
}

func BenchmarkSmallUnmarshal(b *testing.B) {
	t := readCorpusItemFile(pprofSmall, benchWithoutGzip)
	now := time.Now()
	for i := 0; i < b.N; i++ {
		parser := pprof.NewParser(pprof.ParserConfig{SampleTypes: t.config, Putter: putter})
		err := parser.ParsePprof(context.TODO(), now, now, t.profile, false)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBigUnmarshal(b *testing.B) {
	t := readCorpusItemFile(pprofBig, benchWithoutGzip)
	now := time.Now()
	for i := 0; i < b.N; i++ {
		parser := pprof.NewParser(pprof.ParserConfig{SampleTypes: t.config, Putter: putter})
		err := parser.ParsePprof(context.TODO(), now, now, t.profile, false)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCorpus(b *testing.B) {
	corpus := readCorpus(benchmarkCorps, benchWithoutGzip)
	n := benchmarkCorpusSize
	for _, testType := range streamingTestTypes {
		for i := 0; i < n; i++ {
			j := i
			b.Run(fmt.Sprintf("BenchmarkCorpus_%d_pool_%v_arena_%v", j, testType.pool, testType.arenas),
				func(b *testing.B) {
					t := corpus[j]
					benchmarkStreamingOne(b, t, testType)
				})
		}
	}
}

func benchmarkStreamingOne(b *testing.B, t *testcase, testType streamingTestType) {
	now := time.Now()
	for i := 0; i < b.N; i++ {
		config := t.config
		pConfig := streaming.ParserConfig{SampleTypes: config, Putter: putter, ArenasEnabled: testType.arenas}
		var parser *streaming.VTStreamingParser
		if testType.pool {
			parser = streaming.VTStreamingParserFromPool(pConfig)
		} else {
			parser = streaming.NewStreamingParser(pConfig)
		}
		err := parser.ParsePprof(context.TODO(), now, now, t.profile, false)
		if err != nil {
			b.Fatal(err)
		}
		if testType.pool {
			parser.ResetCache()
			parser.ReturnToPool()
		}
		if testType.arenas {
			parser.FreeArena()
		}
	}
}

var streamingTestTypes = []streamingTestType{
	{pool: false, arenas: false},
	{pool: true, arenas: false},
	{pool: false, arenas: true},
}

type streamingTestType struct {
	pool   bool
	arenas bool
}

type testcase struct {
	profile, prev []byte
	config        map[string]*tree.SampleTypeConfig
	fname         string
	spyname       string
}

func readCorpus(dir string, doDecompress bool) []*testcase {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		print(err)
		return nil
	}
	var res []*testcase
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".txt") {
			res = append(res, readCorpusItem(dir, file, doDecompress))
		}
	}
	return res
}

func readCorpusItem(dir string, file fs.FileInfo, doDecompress bool) *testcase {
	fname := dir + "/" + file.Name()
	return readCorpusItemFile(fname, doDecompress)
}

func readCorpusItemFile(fname string, doDecompress bool) *testcase {
	bs, err := ioutil.ReadFile(fname)
	if err != nil {
		panic(err)
	}
	r, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(bs)))
	if err != nil {
		panic(err)
	}
	contentType := r.Header.Get("Content-Type")
	rawData, _ := ioutil.ReadAll(r.Body)
	decompress := func(b []byte) []byte {
		if len(b) < 2 {
			return b
		}
		if b[0] == 0x1f && b[1] == 0x8b {
			gzipr, err := gzip.NewReader(bytes.NewReader(b))
			if err != nil {
				panic(err)
			}
			defer gzipr.Close()
			var buf bytes.Buffer
			if _, err = io.Copy(&buf, gzipr); err != nil {
				panic(err)
			}
			return buf.Bytes()
		}
		return b
	}

	if contentType == "binary/octet-stream" {
		return &testcase{
			profile: decompress(rawData),
			config:  tree.DefaultSampleTypeMapping,
			fname:   fname,
		}
	}
	boundary, err := form.ParseBoundary(contentType)
	if err != nil {
		panic(err)
	}

	f, err := multipart.NewReader(bytes.NewReader(rawData), boundary).ReadForm(32 << 20)
	if err != nil {
		panic(err)
	}
	const (
		formFieldProfile          = "profile"
		formFieldPreviousProfile  = "prev_profile"
		formFieldSampleTypeConfig = "sample_type_config"
	)

	Profile, err := form.ReadField(f, formFieldProfile)
	if err != nil {
		panic(err)
	}
	PreviousProfile, err := form.ReadField(f, formFieldPreviousProfile)
	if err != nil {
		panic(err)
	}

	stBS, err := form.ReadField(f, formFieldSampleTypeConfig)
	if err != nil {
		panic(err)
	}
	var config map[string]*tree.SampleTypeConfig
	if stBS != nil {
		if err = json.Unmarshal(stBS, &config); err != nil {
			panic(err)
		}
	} else {
		config = tree.DefaultSampleTypeMapping
	}
	_ = Profile
	_ = PreviousProfile

	if doDecompress {
		Profile = decompress(Profile)
		PreviousProfile = decompress(PreviousProfile)
	}
	elem := &testcase{Profile, PreviousProfile, config, fname, "gospy"}
	return elem
}

func testCompareOne(t *testing.T, c *testcase, typ streamingTestType) {
	err := pprof.DecodePool(bytes.NewReader(c.profile), func(profile *tree.Profile) error {
		return nil
	})
	fmt.Println(c.fname)
	key, _ := segment.ParseKey("foo.bar")
	mock1 := &MockPutter{keep: true}
	profile1 := pprof.RawProfile{
		Profile:             c.profile,
		PreviousProfile:     c.prev,
		SampleTypeConfig:    c.config,
		StreamingParser:     true,
		PoolStreamingParser: typ.pool,
		ArenasEnabled:       typ.arenas,
	}

	err2 := profile1.Parse(context.TODO(), mock1, nil, ingestion.Metadata{Key: key, SpyName: c.spyname})
	if err2 != nil {
		t.Fatal(err2)
	}

	mock2 := &MockPutter{keep: true}
	profile2 := pprof.RawProfile{
		Profile:          c.profile,
		PreviousProfile:  c.prev,
		SampleTypeConfig: c.config,
	}
	err = profile2.Parse(context.TODO(), mock2, nil, ingestion.Metadata{Key: key, SpyName: c.spyname})
	if err != nil {
		t.Fatal(err)
	}

	if len(mock1.puts) != len(mock2.puts) {
		t.Fatalf("put mismatch %d %d", len(mock1.puts), len(mock2.puts))
	}
	sort.Slice(mock1.puts, func(i, j int) bool {
		return strings.Compare(mock1.puts[i].Key, mock1.puts[j].Key) < 0
	})
	sort.Slice(mock2.puts, func(i, j int) bool {
		return strings.Compare(mock2.puts[i].Key, mock2.puts[j].Key) < 0
	})
	writeGlod := false
	checkGold := true
	trees := map[string]string{}
	gold := c.fname + ".gold.json"
	if checkGold {
		goldBS, err := os.ReadFile(gold)
		if err != nil {
			panic(err)
		}
		err = json.Unmarshal(goldBS, &trees)
		if err != nil {
			panic(err)
		}
	}
	for i := range mock1.puts {
		p1 := mock1.puts[i]
		p2 := mock2.puts[i]
		k1 := p1.Key
		k2 := p2.Key
		if k1 != k2 {
			t.Fatalf("key mismatch %s %s", k1, k2)
		}
		it := p1.Val
		jit := mock2.puts[i].Val

		if it != jit {
			fmt.Println(key.SegmentKey())
			t.Fatalf("mismatch\n --- actual:\n"+
				"%s\n"+
				" --- exopected\n"+
				"%s\n====", it, jit)
		}
		if checkGold {
			git := trees[k1]
			if it != git {
				t.Fatalf("mismatch ---\n"+
					"%s\n"+
					"---\n"+
					"%s\n====", it, git)
			}
		}
		fmt.Printf("ok %s %d \n", k1, len(it))
		if p1.StartTime != p2.StartTime {
			t.Fatal()
		}
		if p1.EndTime != p2.EndTime {
			t.Fatal()
		}
		if p1.Units != p2.Units {
			t.Fatal()
		}
		if p1.AggregationType != p2.AggregationType {
			t.Fatal()
		}
		if p1.SpyName != p2.SpyName {
			t.Fatal()
		}
		if p1.SampleRate != p2.SampleRate {
			t.Fatal()
		}
		if writeGlod {
			trees[k1] = it
		}
	}
	if writeGlod {
		marshal, err := json.Marshal(trees)
		if err != nil {
			panic(err)
		}
		err = os.WriteFile(gold, marshal, 0666)
		if err != nil {
			panic(err)
		}
	}
}

type PutInputCopy struct {
	Val string
	Key string

	StartTime       time.Time
	EndTime         time.Time
	SpyName         string
	SampleRate      uint32
	Units           metadata.Units
	AggregationType metadata.AggregationType
}

type MockPutter struct {
	keep bool
	puts []PutInputCopy
}

func (m *MockPutter) Put(_ context.Context, input *storage.PutInput) error {
	if m.keep {
		m.puts = append(m.puts, PutInputCopy{
			Val:             input.Val.String(),
			Key:             input.Key.SegmentKey(),
			StartTime:       input.StartTime,
			EndTime:         input.EndTime,
			SpyName:         input.SpyName,
			SampleRate:      input.SampleRate,
			Units:           input.Units,
			AggregationType: input.AggregationType,
		})
	}
	return nil
}
