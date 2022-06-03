package remotewrite

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"

	"github.com/pyroscope-io/pyroscope/pkg/parser"
	"github.com/sirupsen/logrus"
)

var ErrUnsupportedFormat = errors.New("unsupported format")
var ErrPprofRequiresPrevProfile = errors.New("pprof requires a prev_profile")

type BodyCreator struct {
	logger *logrus.Logger
}

func NewBodyCreator(logger *logrus.Logger) *BodyCreator {
	return &BodyCreator{
		logger: logger,
	}
}

// Add given a parser.Input adds the payload body to the request
func (b BodyCreator) Add(pi *parser.PutInput) (body io.Reader, contentType string, err error) {
	switch pi.Format {
	case "pprof":
		return b.pprof(pi)
	case "trie":
		return b.trie(pi)
	case "tree":
		return b.tree(pi)
	case "lines":
		return b.lines(pi)
	default:
		return nil, "", ErrUnsupportedFormat
	}
}

func (BodyCreator) trie(pi *parser.PutInput) (bodyReader io.Reader, contentType string, err error) {
	return pi.Profile, "binary/octet-stream+trie", nil
}
func (BodyCreator) tree(pi *parser.PutInput) (bodyReader io.Reader, contentType string, err error) {
	return pi.Profile, "binary/octet-stream+tree", nil
}
func (BodyCreator) lines(pi *parser.PutInput) (bodyReader io.Reader, contentType string, err error) {
	return pi.Profile, "binary/octet-stream+lines", nil
}

func (BodyCreator) pprof(pi *parser.PutInput) (bodyReader io.Reader, contentType string, err error) {
	// TODO(eh-am): is this correct?
	// prev profile should be required only for cumulative profiles
	// so it should not be required for eg cpu
	if pi.PreviousProfile == nil {
		return nil, "", ErrPprofRequiresPrevProfile
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fw, err := writer.CreateFormFile("profile", "profile.pprof")
	fw.Write(streamToByte(pi.Profile))
	if err != nil {
		return nil, "", err
	}

	fw, err = writer.CreateFormFile("prev_profile", "profile.pprof")
	fw.Write(streamToByte(pi.PreviousProfile))
	if err != nil {
		return nil, "", err
	}
	writer.Close()

	return body, writer.FormDataContentType(), nil
}
