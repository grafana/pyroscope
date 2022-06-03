package inout_test

import (
	"bytes"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/inout"
	"github.com/pyroscope-io/pyroscope/pkg/parser"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
)

var _ = Describe("In", func() {
	It("works", func() {
		req, err := http.NewRequest("POST", "https://pyroscope.io", bytes.NewReader([]byte("foo;bar\nfoo;bar\nfoo;baz\nfoo;baz\nfoo;baz\n")))
		Expect(err).NotTo(HaveOccurred())

		req.Header.Set("Content-Type", "binary/octet-stream+lines")

		params := req.URL.Query()
		params.Set("format", "lines")
		params.Set("name", "myapp")
		params.Set("from", "1654110240")
		params.Set("until", "1654110250")
		params.Set("sampleRate", "100")
		params.Set("spyName", "myspy")
		params.Set("units", "samples")
		params.Set("aggregationType", "sum")
		req.URL.RawQuery = params.Encode()

		io := inout.NewInOut()
		pi, err := io.PutInputFromRequest(req)
		Expect(err).NotTo(HaveOccurred())

		Expect(pi.Format).To(Equal(parser.Lines))
		Expect(pi.Key.Normalized()).To(Equal("myapp{}"))
		Expect(pi.StartTime).To(Equal(attime.Parse("1654110240")))
		Expect(pi.EndTime).To(Equal(attime.Parse("1654110250")))
		Expect(pi.SampleRate).To(Equal(uint32(100)))
		Expect(pi.Units).To(Equal(metadata.SamplesUnits))
		Expect(pi.AggregationType).To(Equal(metadata.SumAggregationType))
	})
})
