package inout_test

import (
	"bytes"
	"io"

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
		profile := []byte("foo;bar\nfoo;bar\nfoo;baz\nfoo;baz\nfoo;baz\n")

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
			Profile:         bytes.NewReader(profile),
		}

		inout := inout.NewInOut()

		// Convert to Request
		req, err := inout.RequestFromPutInput(pi, "https://pyroscope.io")
		Expect(err).NotTo(HaveOccurred())

		// Then convert it back to PutInput
		out, err := inout.PutInputFromRequest(req)
		Expect(err).NotTo(HaveOccurred())

		// TODO(eh-am): do this with reflection?
		Expect(out.SpyName).To(Equal(pi.SpyName))
		Expect(out.AggregationType).To(Equal(pi.AggregationType))
		Expect(out.StartTime).To(Equal(pi.StartTime))
		Expect(out.EndTime).To(Equal(pi.EndTime))
		Expect(out.Format).To(Equal(pi.Format))
		Expect(readProfile(len(profile), out.Profile)).To(Equal(profile))
	})
})

func streamToByte(stream io.Reader) []byte {
	buf := new(bytes.Buffer)
	buf.ReadFrom(stream)
	return buf.Bytes()
}
