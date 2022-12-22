package bench

import (
	"context"
	"fmt"
	"github.com/pyroscope-io/pyroscope/pkg/convert/pprof"
	"github.com/pyroscope-io/pyroscope/pkg/convert/pprof/streaming"
	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

//todo test protobufs from our go,ruby,dotnet integrations

type MockPutter struct {
	keep bool
	puts []*storage.PutInput
}

func (m *MockPutter) Put(ctx context.Context, input *storage.PutInput) error {
	if m.keep {
		m.puts = append(m.puts, input)
	}
	//fmt.Printf("put \n%v\n", input.Val)

	return nil
}

var putter = &MockPutter{}

func TestStreamingParser(t *testing.T) {
	terstcases := readCorpus("/home/korniltsev/Downloads/pprofs_short")
	//p, err := os.ReadFile("../testdata/heap.pb.gz")
	//if err != nil {
	//	t.Fatal(err)
	//}
	for _, terstcase := range terstcases {
		parser := streaming.NewStreamingParser(
			streaming.ParserConfig{
				SampleTypes: terstcase.config,
				Putter:      putter,
			})
		parser.ParsePprof(context.TODO(), time.Now(), time.Now(), terstcase.profile)
	}

}

func TestUmarshalParser(t *testing.T) {
	terstcases := readCorpus("/home/korniltsev/Downloads/pprofs_short")
	//p, err := os.ReadFile("../testdata/heap.pb.gz")
	//if err != nil {
	//	t.Fatal(err)
	//}
	for _, terstcase := range terstcases {
		parser := pprof.NewParser(
			pprof.ParserConfig{
				SampleTypes: terstcase.config,
				Putter:      putter,
			})
		parser.ParsePprof(context.TODO(), time.Now(), time.Now(), terstcase.profile)
	}
}

func TestStreamingParserOne(t *testing.T) {

	p, err := os.ReadFile("../testdata/heap.pb.gz")
	if err != nil {
		t.Fatal(err)
	}

	parser := streaming.NewStreamingParser(
		streaming.ParserConfig{
			SampleTypes: tree.DefaultSampleTypeMapping,
			Putter:      putter,
		})
	parser.ParsePprof(context.TODO(), time.Now(), time.Now(), p)

}

func TestUmarshalParserOne(t *testing.T) {

	p, err := os.ReadFile("../testdata/heap.pb.gz")
	if err != nil {
		t.Fatal(err)
	}

	parser := pprof.NewParser(
		pprof.ParserConfig{
			SampleTypes: tree.DefaultSampleTypeMapping,
			Putter:      putter,
		})
	parser.ParsePprof(context.TODO(), time.Now(), time.Now(), p)

}

func TestCompare(t *testing.T) {

	for _, c := range readCorpus("/home/korniltsev/Downloads/pprofs_short") {
		testOne(t, c)
	}

	for _, c := range readCorpus("/home/korniltsev/Downloads/pprofs") {
		testOne(t, c)
	}
}

func testOne(t *testing.T, c *testcase) {

	key, _ := segment.ParseKey("foo.bar")
	mock1 := &MockPutter{keep: true}
	profile1 := pprof.RawProfile{
		Profile:          c.profile,
		PreviousProfile:  c.prev,
		SampleTypeConfig: c.config,
		StreamingParser:  true,
	}
	if c.prev != nil {
		fmt.Println("foo")
	}
	profile1.Parse(context.TODO(), mock1, nil, ingestion.Metadata{Key: key})

	mock2 := &MockPutter{keep: true}
	profile2 := pprof.RawProfile{
		Profile:          c.profile,
		PreviousProfile:  c.prev,
		SampleTypeConfig: c.config,
	}
	err := profile2.Parse(context.TODO(), mock2, nil, ingestion.Metadata{Key: key})
	if err != nil {
		t.Fatal(err)
	}

	if len(mock1.puts) != len(mock2.puts) {
		t.Fatalf("put mismatch")
	}
	sort.Slice(mock1.puts, func(i, j int) bool {
		return strings.Compare(mock1.puts[i].Key.SegmentKey(), mock1.puts[j].Key.SegmentKey()) < 0
	})
	sort.Slice(mock2.puts, func(i, j int) bool {
		return strings.Compare(mock2.puts[i].Key.SegmentKey(), mock2.puts[j].Key.SegmentKey()) < 0
	})
	for i := range mock1.puts {
		k1 := mock1.puts[i].Key.SegmentKey()
		k2 := mock2.puts[i].Key.SegmentKey()
		if k1 != k2 {
			t.Fatalf("key mismatch %s %s", k1, k2)
		}
		it := mock1.puts[i].Val.String()
		jit := mock2.puts[i].Val.String()
		if it != jit {
			t.Fatalf("mismatch ---\n"+
				"%s\n"+
				"---\n"+
				"%s\n====", it, jit)
		}
		fmt.Printf("ok %s %d \n", k1, len(it))
	}
}
