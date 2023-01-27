package bench

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/pprof/profile"
	"github.com/pyroscope-io/pyroscope/pkg/convert/pprof"
	"github.com/pyroscope-io/pyroscope/pkg/convert/pprof/streaming"
	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
	"github.com/pyroscope-io/pyroscope/pkg/stackbuilder"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/util/form"
	"golang.org/x/exp/slices"
	"io"
	"io/fs"
	"math/big"
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

var compareCorpusData = readCorpus(compareCorpus, benchWithoutGzip)

func TestCompare(t *testing.T) {
	if len(compareCorpusData) == 0 {
		t.Skip("empty corpus")
		return
	}
	for _, testType := range streamingTestTypes {
		t.Run(fmt.Sprintf("TestCompare_pool_%v_arenas_%v", testType.pool, testType.arenas), func(t *testing.T) {
			for _, c := range compareCorpusData {
				testCompareOne(t, c, testType)
			}
		})
	}
}

func TestCompareWriteBatch(t *testing.T) {
	if len(compareCorpusData) == 0 {
		t.Skip("empty corpus")
		return
	}
	for _, c := range compareCorpusData {
		//cur, _ := profile.Parse(bytes.NewReader(c.profile))
		//if c.prev != nil {
		//	prev, _ := profile.Parse(bytes.NewReader(c.prev))
		//	os.WriteFile("p1", []byte(dumpPProfProfile(prev)), 0666)
		//}
		//os.WriteFile("p2", []byte(dumpPProfProfile(cur)), 0666)
		testCompareWriteBatchOne(t, c)
	}
}

func dumpPProfProfile(p *profile.Profile) string {
	var ls []string
	for _, sample := range p.Sample {
		s := dumpPProfStack(sample, true)
		ls = append(ls, s)
	}
	slices.Sort(ls)
	return strings.Join(ls, "\n")
}

func dumpPProfStack(sample *profile.Sample, v bool) string {
	sb := strings.Builder{}
	for i := len(sample.Location) - 1; i >= 0; i-- {
		location := sample.Location[i]
		for j := len(location.Line) - 1; j >= 0; j-- {
			line := location.Line[j]

			sb.WriteString(";")
			//sb.WriteString(fmt.Sprintf("[%x %x] ", location.ID, location.Address))

			sb.WriteString(line.Function.Name)
		}
	}
	if v {
		sb.WriteString(" ")
		sb.WriteString(fmt.Sprintf("%d", sample.Value[0]))
	}
	s := sb.String()
	return s
}

func TestIterateWithStackBuilder(t *testing.T) {
	sb := newStackBuilder()
	it := tree.New()
	it.Insert([]byte(""), uint64(43))
	it.Insert([]byte("a"), uint64(42))
	it.Insert([]byte("a;b"), uint64(1))
	it.Insert([]byte("a;c"), uint64(2))
	it.Insert([]byte("a;d;e"), uint64(3))
	it.Insert([]byte("a;d;f"), uint64(4))

	it.IterateWithStackBuilder(sb, func(stackID uint64, v uint64) {
		sb.stackID2Val[stackID] = v
	})
	sb.expectValue(t, 0, 43)
	sb.expectValue(t, 1, 42)
	sb.expectValue(t, 2, 1)
	sb.expectValue(t, 3, 2)
	sb.expectValue(t, 4, 3)
	sb.expectValue(t, 5, 4)
	sb.expectStack(t, 0, "")
	sb.expectStack(t, 1, "a")
	sb.expectStack(t, 2, "a;b")
	sb.expectStack(t, 3, "a;c")
	sb.expectStack(t, 4, "a;d;e")
	sb.expectStack(t, 5, "a;d;f")
}

func TestIterateWithStackBuilderEmpty(t *testing.T) {
	it := tree.New()
	sb := newStackBuilder()
	it.IterateWithStackBuilder(sb, func(stackID uint64, v uint64) {
		t.Fatal()
	})
}

func newStackBuilder() *mockStackBuilder {
	return &mockStackBuilder{
		stackID2Stack:      make(map[uint64]string),
		stackID2Val:        make(map[uint64]uint64),
		stackID2StackBytes: make(map[uint64][][]byte),
	}
}

func TestTreeIterationCorpus(t *testing.T) {
	corpus := readCorpus(compareCorpus, benchWithoutGzip)
	if len(corpus) == 0 {
		t.Skip("empty corpus")
		return
	}
	for _, c := range corpus {
		key, _ := segment.ParseKey("foo.bar")
		mock1 := &MockPutter{keep: true}
		profile1 := pprof.RawProfile{
			Profile:             c.profile,
			PreviousProfile:     c.prev,
			SampleTypeConfig:    c.config,
			StreamingParser:     true,
			PoolStreamingParser: true,
			ArenasEnabled:       false,
		}

		err2 := profile1.Parse(context.TODO(), mock1, nil, ingestion.Metadata{Key: key, SpyName: c.spyname})
		if err2 != nil {
			t.Fatal(err2)
		}
		for _, put := range mock1.puts {
			testIterateOne(t, put.ValTree)
		}
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

func testCompareWriteBatchOne(t *testing.T, c *testcase) {
	fmt.Println(c.fname)
	key, _ := segment.ParseKey("foo.bar")
	profile1 := pprof.RawProfile{
		Profile:          c.profile,
		PreviousProfile:  c.prev,
		SampleTypeConfig: c.config,
	}
	md := ingestion.Metadata{Key: key, SpyName: c.spyname}
	wbf := &mockWriteBatchFactory{}
	err := profile1.ParseWithWriteBatch(context.TODO(), wbf, md)
	if err != nil {
		t.Fatal(err)
	}

	mock2 := &MockPutter{keep: true}
	profile2 := &pprof.RawProfile{
		Profile:          c.profile,
		PreviousProfile:  c.prev,
		SampleTypeConfig: c.config,
	}
	mergeCumulative(profile2)

	err = profile2.Parse(context.TODO(), mock2, nil, md)
	if err != nil {
		t.Fatal(err)
	}

	for _, put := range mock2.puts {
		expectedCollapsed := put.Val
		appenderCollapsed := ""
		var found []*mockSamplesAppender
		for _, batch := range wbf.wbs {
			for _, appender := range batch.appenders {
				labels := make(map[string]string)
				labels["__name__"] = batch.appName
				for _, label := range appender.labels {
					labels[label.Key] = label.Value
				}
				k := segment.NewKey(labels)
				if k.SegmentKey() == put.Key {
					found = append(found, appender)
				}
			}
		}
		if len(found) != 1 {
			if expectedCollapsed == "" {
				continue
			}
			t.Fatalf("not found %s", put.Key)
		}
		appenderCollapsed = found[0].tree.String()

		if appenderCollapsed != expectedCollapsed {
			os.WriteFile("p3", []byte(expectedCollapsed), 0666)
			os.WriteFile("p4", []byte(appenderCollapsed), 0666)
			t.Fatalf("%s: expected\n%s\ngot\n%s\n failed file:%s\n", put.Key, expectedCollapsed, appenderCollapsed, c.fname)
		}
	}
}

func mergeCumulative(profile2 *pprof.RawProfile) {
	if profile2.PreviousProfile != nil {
		p1, _ := profile.Parse(bytes.NewReader(profile2.PreviousProfile))
		p2, _ := profile.Parse(bytes.NewReader(profile2.Profile))
		prev := []map[string]int64{
			make(map[string]int64),
			make(map[string]int64),
		}
		for _, sample := range p1.Sample {
			s := dumpPProfStack(sample, false)
			prev[0][s] += sample.Value[0]
			prev[1][s] += sample.Value[1]
		}
		dec := func(s string, i int, v int64) int64 {
			prevV := prev[i][s]
			if v > prevV {
				prev[i][s] = 0
				return v - prevV
			}
			prev[i][s] = prevV - v
			return 0
		}
		for _, sample := range p2.Sample {
			s := dumpPProfStack(sample, false)
			sample.Value[0] = dec(s, 0, sample.Value[0])
			sample.Value[1] = dec(s, 1, sample.Value[1])
		}

		merged := p2.Compact()

		bs := bytes.NewBuffer(nil)
		merged.Write(bs)

		profile2.PreviousProfile = nil
		profile2.Profile = bs.Bytes()

		sampleTypeConfig := make(map[string]*tree.SampleTypeConfig)
		for k, v := range profile2.SampleTypeConfig {
			vv := *v
			vv.Cumulative = false
			sampleTypeConfig[k] = &vv
		}
		profile2.SampleTypeConfig = sampleTypeConfig
		//os.WriteFile("merged", []byte(dumpPProfProfile(merged)), 0666)
	}
}

func testIterateOne(t *testing.T, pt *tree.Tree) {
	sb := newStackBuilder()
	var lines []string
	pt.IterateWithStackBuilder(sb, func(stackID uint64, val uint64) {
		lines = append(lines, fmt.Sprintf("%s %d", sb.stackID2Stack[stackID], val))
	})
	s := pt.String()
	s = pt.String()
	var expectedLines []string
	if s != "" {
		expectedLines = strings.Split(strings.Trim(s, "\n"), "\n")
	}
	slices.Sort(lines)
	slices.Sort(expectedLines)
	if !slices.Equal(lines, expectedLines) {
		expected := strings.Join(expectedLines, "\n")
		got := strings.Join(lines, "\n")
		t.Fatalf("expected %v got\n%v", expected, got)
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
	ValTree         *tree.Tree
}

type MockPutter struct {
	keep bool
	puts []PutInputCopy
}

func (m *MockPutter) Put(_ context.Context, input *storage.PutInput) error {
	if m.keep {
		m.puts = append(m.puts, PutInputCopy{
			Val:             input.Val.String(),
			ValTree:         input.Val.Clone(big.NewRat(1, 1)),
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

type mockStackBuilder struct {
	ss [][]byte

	stackID2Stack      map[uint64]string
	stackID2StackBytes map[uint64][][]byte
	stackID2Val        map[uint64]uint64
}

func (s *mockStackBuilder) Push(frame []byte) {
	s.ss = append(s.ss, frame)
}

func (s *mockStackBuilder) Pop() {
	s.ss = s.ss[0 : len(s.ss)-1]
}

func (s *mockStackBuilder) Build() (stackID uint64) {
	res := ""
	for _, bs := range s.ss {
		if len(res) != 0 {
			res += ";"
		}
		res += string(bs)
	}
	id := uint64(len(s.stackID2Stack))
	s.stackID2Stack[id] = res

	bs := make([][]byte, 0, len(s.ss))
	for _, frame := range s.ss {
		bs = append(bs, append([]byte{}, frame...))
	}
	s.stackID2StackBytes[id] = bs
	return id
}

func (s *mockStackBuilder) Reset() {
	s.ss = s.ss[:0]
}

func (s *mockStackBuilder) expectValue(t *testing.T, stackID, expected uint64) {
	if s.stackID2Val[stackID] != expected {
		t.Fatalf("expected at %d %d got %d", stackID, expected, s.stackID2Val[stackID])
	}
}
func (s *mockStackBuilder) expectStack(t *testing.T, stackID uint64, expected string) {
	if s.stackID2Stack[stackID] != expected {
		t.Fatalf("expected at %d %s got %s", stackID, expected, s.stackID2Stack[stackID])
	}
}

type mockWriteBatchFactory struct {
	wbs map[string]*mockWriteBatch
}

func (m *mockWriteBatchFactory) NewWriteBatch(appName string) (stackbuilder.WriteBatch, error) {
	if m.wbs == nil {
		m.wbs = make(map[string]*mockWriteBatch)
	}
	if m.wbs[appName] != nil {
		panic("already exists")
	}
	wb := &mockWriteBatch{
		appName:   appName,
		sb:        newStackBuilder(),
		appenders: make(map[string]*mockSamplesAppender),
	}
	m.wbs[appName] = wb
	return wb, nil
}

type mockWriteBatch struct {
	appName   string
	sb        *mockStackBuilder
	appenders map[string]*mockSamplesAppender
}

func (m *mockWriteBatch) StackBuilder() tree.StackBuilder {
	return m.sb
}

func (m *mockWriteBatch) SamplesAppender(startTime, endTime int64, labels stackbuilder.Labels) stackbuilder.SamplesAppender {
	sLabels, _ := json.Marshal(labels)
	k := fmt.Sprintf("%d-%d-%s", startTime, endTime, sLabels)
	a := m.appenders[k]
	if a != nil {
		return a
	}
	a = &mockSamplesAppender{
		startTime: startTime,
		endTime:   endTime,
		labels:    labels,
		sb:        m.sb,
	}
	m.appenders[k] = a
	return a
}

func (*mockWriteBatch) Flush() {

}

type mockSamplesAppender struct {
	startTime, endTime int64
	labels             stackbuilder.Labels
	stacks             []stackIDToVal
	tree               *tree.Tree
	sb                 *mockStackBuilder
}

type stackIDToVal struct {
	stackID uint64
	val     uint64
}

func (m *mockSamplesAppender) Append(stackID, value uint64) {
	m.stacks = append(m.stacks, stackIDToVal{stackID, value})
	stack := m.sb.stackID2StackBytes[stackID]
	if stack == nil {
		panic("not found")
	}
	if m.tree == nil {
		m.tree = tree.New()
	}
	m.tree.InsertStack(stack, value)
}
