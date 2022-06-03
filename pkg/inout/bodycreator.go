package inout

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"

	"github.com/pyroscope-io/pyroscope/pkg/parser"
)

var ErrUnsupportedFormat = errors.New("unsupported format")
var ErrPprofRequiresPrevProfile = errors.New("pprof requires a prev_profile")

type bodyCreator struct{}

func (b bodyCreator) Create(pi *parser.PutInput) (body io.Reader, contentType string, err error) {
	switch pi.Format {
	case "pprof":
		return b.pprof(pi)
	case "trie":
		return b.trie(pi)
	case "tree":
		return b.tree(pi)
	case "lines":
		return b.lines(pi)
	case "jfr":
		return b.jfr(pi)
	default:
		return nil, "", ErrUnsupportedFormat
	}
}

func (bodyCreator) trie(pi *parser.PutInput) (bodyReader io.Reader, contentType string, err error) {
	return pi.Profile, "binary/octet-stream+trie", nil
}
func (bodyCreator) tree(pi *parser.PutInput) (bodyReader io.Reader, contentType string, err error) {
	return pi.Profile, "binary/octet-stream+tree", nil
}
func (bodyCreator) lines(pi *parser.PutInput) (bodyReader io.Reader, contentType string, err error) {
	return pi.Profile, "binary/octet-stream+lines", nil
}
func (bodyCreator) pprof(pi *parser.PutInput) (bodyReader io.Reader, contentType string, err error) {
	// TODO(eh-am): is this correct?
	// prev profile should be required only for cumulative profiles
	// so it should not be required for eg cpu

	// also TODO(eh-am): support https://pyroscope.io/docs/server-api-reference/#sample-type-configuration
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

func (bodyCreator) jfr(pi *parser.PutInput) (bodyReader io.Reader, contentType string, err error) {
	return pi.Profile, "application/x-www-form-urlencoded", nil
}

func streamToByte(stream io.Reader) []byte {
	buf := new(bytes.Buffer)
	buf.ReadFrom(stream)
	return buf.Bytes()
}
