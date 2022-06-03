package inout_test

import (
	"bytes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/inout"
	"github.com/pyroscope-io/pyroscope/pkg/parser"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
)

var _ = Describe("In/Out Integration", func() {
	It("works", func() {
		pi := &parser.PutInput{
			Key: segment.NewKey(map[string]string{
				"__name__": "myapp",
			}),

			Format:          parser.Lines,
			StartTime:       attime.Parse("1654110240"),
			EndTime:         attime.Parse("1654110250"),
			SampleRate:      100,
			SpyName:         "gospy",
			Units:           metadata.SamplesUnits,
			AggregationType: metadata.SumAggregationType,
			Profile:         bytes.NewReader([]byte("foo;bar\nfoo;bar\nfoo;baz\nfoo;baz\nfoo;baz\n")),
		}

		inout := inout.NewInOut()

		// Convert to Request
		req, err := inout.RequestFromPutInput(pi, "https://pyroscope.io")
		Expect(err).NotTo(HaveOccurred())

		// Then convert it back to PutInput
		out, err := inout.PutInputFromRequest(req)
		Expect(err).NotTo(HaveOccurred())

		// TODO(eh-am): check fields individually since tructs are likely not the same
		Expect(*out).To(Equal(*pi))
	})
})
