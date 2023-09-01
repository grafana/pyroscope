package jfr

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/grafana/pyroscope/pkg/og/convert/pprof/bench"
	"github.com/grafana/pyroscope/pkg/og/storage"
	"github.com/grafana/pyroscope/pkg/og/storage/segment"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestParser(t *testing.T) {
	RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Java JFR Parser suite")
}

func TestParseCompareExpectedData(t *testing.T) {
	testdata := []struct {
		jfr    string
		labels string
	}{
		{"testdata/cortex-dev-01__kafka-0__cpu__0.jfr.gz", ""},
		{"testdata/cortex-dev-01__kafka-0__cpu__1.jfr.gz", ""},
		{"testdata/cortex-dev-01__kafka-0__cpu__2.jfr.gz", ""},
		{"testdata/cortex-dev-01__kafka-0__cpu__3.jfr.gz", ""},
		{"testdata/cortex-dev-01__kafka-0__cpu_lock_alloc__0.jfr.gz", ""},
		{"testdata/cortex-dev-01__kafka-0__cpu_lock_alloc__1.jfr.gz", ""},
		{"testdata/cortex-dev-01__kafka-0__cpu_lock_alloc__2.jfr.gz", ""},
		{"testdata/cortex-dev-01__kafka-0__cpu_lock_alloc__3.jfr.gz", ""},
		{"testdata/cortex-dev-01__kafka-0__cpu_lock0_alloc0__0.jfr.gz", ""},
		{"testdata/dump1.jfr.gz", "testdata/dump1.labels.pb.gz"},
	}
	for _, td := range testdata {
		t.Run(td.jfr, func(t *testing.T) {
			jfr, err := bench.ReadGzipFile(td.jfr)
			require.NoError(t, err)
			putter := &bench.MockPutter{Keep: true}
			k, err := segment.ParseKey("kafka.app")
			require.NoError(t, err)

			pi := &storage.PutInput{
				StartTime:  time.UnixMilli(1000),
				EndTime:    time.UnixMilli(2000),
				Key:        k,
				SpyName:    "java",
				SampleRate: 100,
			}
			var labels = new(LabelsSnapshot)
			if td.labels != "" {
				labelsBytes, err := bench.ReadGzipFile(td.labels)
				require.NoError(t, err)
				err = proto.Unmarshal(labelsBytes, labels)
				require.NoError(t, err)
			}
			err = ParseJFR(context.TODO(), putter, jfr, pi, labels)
			require.NoError(t, err)
			jsonFile := strings.TrimSuffix(td.jfr, ".jfr.gz") + ".json.gz"
			//err = putter.DumpJson(jsonFile)
			err = putter.CompareWithJson(jsonFile)
			require.NoError(t, err)
		})
	}
}

func BenchmarkParser(b *testing.B) {
	tests := []string{
		"testdata/cortex-dev-01__kafka-0__cpu__0.jfr.gz",
		"testdata/cortex-dev-01__kafka-0__cpu__1.jfr.gz",
		"testdata/cortex-dev-01__kafka-0__cpu__2.jfr.gz",
		"testdata/cortex-dev-01__kafka-0__cpu__3.jfr.gz",
		"testdata/cortex-dev-01__kafka-0__cpu_lock_alloc__0.jfr.gz",
		"testdata/cortex-dev-01__kafka-0__cpu_lock_alloc__1.jfr.gz",
		"testdata/cortex-dev-01__kafka-0__cpu_lock_alloc__2.jfr.gz",
		"testdata/cortex-dev-01__kafka-0__cpu_lock_alloc__3.jfr.gz",
		"testdata/cortex-dev-01__kafka-0__cpu_lock0_alloc0__0.jfr.gz",
	}

	for _, testdata := range tests {
		f := testdata
		b.Run(testdata, func(b *testing.B) {
			jfr, err := bench.ReadGzipFile(f)
			require.NoError(b, err)
			k, err := segment.ParseKey("kafka.app")
			require.NoError(b, err)
			pi := &storage.PutInput{
				StartTime:  time.UnixMilli(1000),
				EndTime:    time.UnixMilli(2000),
				Key:        k,
				SpyName:    "java",
				SampleRate: 100,
			}
			putter := &bench.MockPutter{Keep: false}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				err = ParseJFR(context.TODO(), putter, jfr, pi, nil)
			}
		})
	}
}
