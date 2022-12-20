package pprof_test

import (
	"context"
	"github.com/pyroscope-io/pyroscope/pkg/convert/pprof"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConvert(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "pprof Suite")
}





//todo remove
func TestUnmarshalParser(t *testing.T) {
	p, err := os.ReadFile("testdata/heap.pb.gz")
	if err != nil {
		t.Fatal(err)
	}

	parser := pprof.NewParser(pprof.ParserConfig{SampleTypes: tree.DefaultSampleTypeMapping})
	parser.ParsePprof(context.TODO(), time.Now(), time.Now(), p)
}
