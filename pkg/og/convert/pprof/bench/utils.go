package bench

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/grafana/pyroscope/pkg/og/storage"
	"github.com/grafana/pyroscope/pkg/og/storage/metadata"
	"github.com/grafana/pyroscope/pkg/og/storage/tree"
)

type PutInputCopy struct {
	Val string // tree serialized to collapsed format
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
	Keep bool
	Puts []PutInputCopy

	JsonDump  bool
	JsonCheck bool
	Trees     map[string]string
}

func (m *MockPutter) Sort() {
	sort.Slice(m.Puts, func(i, j int) bool {
		return strings.Compare(m.Puts[i].Key, m.Puts[j].Key) < 0
	})
}

func (m *MockPutter) DumpJson(jsonFile string) error {
	m.Sort()
	m.Trees = make(map[string]string)

	for i := range m.Puts {
		p1 := m.Puts[i]
		k1 := p1.Key
		it := p1.Val

		m.Trees[k1] = it
	}

	marshal, err := json.Marshal(m.Trees)
	if err != nil {
		return err
	}
	return WriteGzipFile(jsonFile, marshal)

}
func (m *MockPutter) CompareWithJson(jsonFile string) error {
	m.Sort()
	goldBS, err := ReadGzipFile(jsonFile)
	if err != nil {
		return err
	}
	m.Trees = make(map[string]string)
	err = json.Unmarshal(goldBS, &m.Trees)
	if err != nil {
		return err
	}

	for i := range m.Puts {
		p1 := m.Puts[i]
		k1 := p1.Key
		it := p1.Val

		git := m.Trees[k1]
		if it != git {
			return fmt.Errorf("json mismatch %s %s", it, git)
		}
		fmt.Printf("%s len %d ok\n", k1, len(it))

	}

	return nil

}

func (m *MockPutter) Put(_ context.Context, input *storage.PutInput) error {
	if m.Keep {
		m.Puts = append(m.Puts, PutInputCopy{
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

func ReadGzipFile(f string) ([]byte, error) {
	fd, err := os.Open(f)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	g, err := gzip.NewReader(fd)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(g)

}

func WriteGzipFile(f string, data []byte) error {
	fd, err := os.OpenFile(f, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer fd.Close()
	g := gzip.NewWriter(fd)
	_, err = g.Write(data)
	if err != nil {
		return err
	}
	return g.Close()

}
