package remotewrite_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"github.com/pyroscope-io/pyroscope/pkg/parser"
	"github.com/pyroscope-io/pyroscope/pkg/remotewrite"
	"github.com/sirupsen/logrus"
)

// TODO(eh-am): unify this with ingest_test.go
func readTestdataFile(name string) []byte {
	f, err := ioutil.ReadFile(name)
	Expect(err).ToNot(HaveOccurred())
	return f
}

var _ = Describe("BodyCreator", func() {
	var logger *logrus.Logger
	var bc *remotewrite.BodyCreator
	var pi *parser.PutInput

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetOutput(ioutil.Discard)

		pi = &parser.PutInput{}
		bc = remotewrite.NewBodyCreator(logger)
	})

	When("format is not supported", func() {
		BeforeEach(func() {
			pi.Format = "unsupported"
		})

		It("fails with ErrUnsupportedFormat", func() {
			_, _, err := bc.Add(pi)
			Expect(err).To(MatchError(remotewrite.ErrUnsupportedFormat))
		})
	})

	When("format is 'pprof'", func() {
		BeforeEach(func() {
			pi.Format = "pprof"
		})

		When("there's both a 'profile' and a 'prev_profile'", func() {
			It("generates the body correctly", func() {
				bufProfile := readTestdataFile("./testdata/profile.pprof")
				bufPrevProfile := readTestdataFile("./testdata/prev_profile.pprof")

				pi.Profile = bytes.NewReader(bufProfile)
				pi.PreviousProfile = bytes.NewReader(bufPrevProfile)
				body, contentType, err := bc.Add(pi)
				Expect(err).NotTo(HaveOccurred())

				_, params, err := mime.ParseMediaType(contentType)
				Expect(err).NotTo(HaveOccurred())
				boundary, _ := params["boundary"]

				// read profile and prev_profile
				form, err := multipart.NewReader(body, boundary).ReadForm(32 << 20)
				Expect(err).NotTo(HaveOccurred())
				profile, err := formField(form, "profile")
				Expect(err).NotTo(HaveOccurred())
				prevProfile, err := formField(form, "prev_profile")
				Expect(err).NotTo(HaveOccurred())

				// check it's the same as in putInput
				b := make([]byte, len(bufProfile))
				gbytes.TimeoutReader(profile, time.Second).Read(b)
				Expect(b).To(Equal(bufProfile))

				// check it's the same as in putInput
				b = make([]byte, len(bufPrevProfile))
				gbytes.TimeoutReader(prevProfile, time.Second).Read(b)
				Expect(b).To(Equal(bufPrevProfile))
			})
		})
	})
})

// TODO(eh-am): this was copied from ingest.go
func formField(form *multipart.Form, name string) (_ io.Reader, err error) {
	files, ok := form.File[name]
	if !ok || len(files) == 0 {
		return nil, nil
	}
	fh := files[0]
	if fh.Size == 0 {
		return nil, nil
	}
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer func() {
		err = f.Close()
	}()
	b := bytes.NewBuffer(make([]byte, 0, fh.Size))
	if _, err = io.Copy(b, f); err != nil {
		return nil, err
	}
	return b, nil
}
