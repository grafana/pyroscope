package remotewrite_test

import (
	"bytes"
	"compress/gzip"
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

func readProfile(size int, profile io.Reader) []byte {
	b := make([]byte, size)
	gbytes.TimeoutReader(profile, time.Second).Read(b)
	return b
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
			_, _, err := bc.Create(pi)
			Expect(err).To(MatchError(remotewrite.ErrUnsupportedFormat))
		})
	})

	When("format is 'pprof'", func() {
		var bufProfile []byte
		var bufPrevProfile []byte

		BeforeEach(func() {
			pi.Format = "pprof"
		})

		When("there's a single profile", func() {
			BeforeEach(func() {
				bufProfile = readTestdataFile("./testdata/profile.pprof")
				pi.Profile = bytes.NewReader(bufProfile)
			})

			It("is not supported", func() {
				_, _, err := bc.Create(pi)
				Expect(err).To(MatchError(remotewrite.ErrPprofRequiresPrevProfile))
			})
		})

		When("there's both a 'profile' and a 'prev_profile'", func() {
			BeforeEach(func() {
				bufProfile = readTestdataFile("./testdata/profile.pprof")
				bufPrevProfile = readTestdataFile("./testdata/prev_profile.pprof")

				pi.Profile = bytes.NewReader(bufProfile)
				pi.PreviousProfile = bytes.NewReader(bufPrevProfile)
			})

			It("generates the body correctly", func() {
				body, contentType, err := bc.Create(pi)
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

	When("format is 'jfr'", func() {

	})

	When("format is trie", func() {
		var buf []byte

		BeforeEach(func() {
			buf = []byte("\x00\x00\x01\x06foo;ba\x00\x02\x01r\x02\x00\x01z\x03\x00")
			pi.Profile = bytes.NewReader(buf)
			pi.Format = "trie"
		})

		It("works", func() {
			body, contentType, err := bc.Create(pi)
			Expect(err).NotTo(HaveOccurred())
			Expect(contentType).To(Equal("binary/octet-stream+trie"))
			Expect(readProfile(len(buf), body)).To(Equal(buf))
		})
	})

	When("format is tree", func() {
		var buf []byte

		BeforeEach(func() {
			buf = []byte("\x00\x00\x01\x03foo\x00\x02\x03bar\x02\x00\x03baz\x03\x00")
			pi.Profile = bytes.NewReader(buf)
			pi.Format = "tree"
		})

		It("works", func() {
			body, contentType, err := bc.Create(pi)
			Expect(err).NotTo(HaveOccurred())
			Expect(contentType).To(Equal("binary/octet-stream+tree"))
			Expect(readProfile(len(buf), body)).To(Equal(buf))
		})
	})

	When("format is lines", func() {
		var buf []byte

		BeforeEach(func() {
			buf = []byte("foo;bar\nfoo;bar\nfoo;baz\nfoo;baz\nfoo;baz\n")
			pi.Profile = bytes.NewReader(buf)
			pi.Format = "lines"
		})

		It("works", func() {
			body, contentType, err := bc.Create(pi)
			Expect(err).NotTo(HaveOccurred())
			Expect(contentType).To(Equal("binary/octet-stream+lines"))
			Expect(readProfile(len(buf), body)).To(Equal(buf))
		})
	})

	When("format is jfr", func() {
		var buf []byte

		BeforeEach(func() {
			buf = jfrFromFile("../server/testdata/jfr.bin.gz").Bytes()
			pi.Profile = bytes.NewReader(buf)
			pi.Format = "jfr"
		})

		It("works", func() {
			body, contentType, err := bc.Create(pi)
			Expect(err).NotTo(HaveOccurred())
			Expect(contentType).To(Equal("application/x-www-form-urlencoded"))
			Expect(readProfile(len(buf), body)).To(Equal(buf))
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

// TODO(eh-am): unify from ingest_test.go
func jfrFromFile(name string) *bytes.Buffer {
	b, err := ioutil.ReadFile(name)
	Expect(err).ToNot(HaveOccurred())
	b2, err := gzip.NewReader(bytes.NewBuffer(b))
	Expect(err).ToNot(HaveOccurred())
	b3, err := io.ReadAll(b2)
	Expect(err).ToNot(HaveOccurred())
	return bytes.NewBuffer(b3)
}
