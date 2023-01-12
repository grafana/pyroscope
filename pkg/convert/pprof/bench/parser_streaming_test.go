package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/pyroscope-io/pyroscope/pkg/convert/pprof"
	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

//todo test protobufs from our go,ruby,dotnet integrations

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

func (m *MockPutter) Put(ctx context.Context, input *storage.PutInput) error {
	if m.keep {
		m.puts = append(m.puts, PutInputCopy{
			Val:       input.Val.String(),
			Key:       input.Key.SegmentKey(),
			StartTime: input.StartTime,
			EndTime:   input.EndTime,
			//SpyName:         string(input.SpyName),
			SpyName:         input.SpyName,
			SampleRate:      input.SampleRate,
			Units:           input.Units,
			AggregationType: input.AggregationType,
		})
	}
	//fmt.Printf("put \n%v\n", input.Val)

	return nil
}

var putter = &MockPutter{}

func TestCompare(t *testing.T) {
	for _, c := range readCorpus("../../../../../cloudstorage/pkg/pyroscope/pprof/testdata") {
		testOne(t, c)
	}
	hs := []string{}
	for _, hsi := range hs {
		for _, c := range readCorpus(hsi) {
			testOne(t, c)
		}
	}

}

func testOne(t *testing.T, c *testcase) {
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
		PoolStreamingParser: false,
		ArenasEnabled:       true,
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
		if p2.EndTime != p2.EndTime {
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
