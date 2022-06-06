package inout

import (
	"bytes"
	"errors"
	"fmt"
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
func (b bodyCreator) pprof(pi *parser.PutInput) (bodyReader io.Reader, contentType string, err error) {
	// When there's 2 profiles (profile and prev_profile), we send as multipart/form-data
	if pi.PreviousProfile != nil {
		return b.pprofMultipart(pi)
	}

	buf := &bytes.Buffer{}
	buf.ReadFrom(pi.Profile)

	println("buf")
	fmt.Println(buf.Bytes())

	fmt.Println("printing again")
	fmt.Println(buf.Bytes())
	//	println(buf.Bytes())
	reader := bytes.NewReader(buf.Bytes())

	// Otherwise, send in the body directly
	//	return pi.Profile, "application/octet-stream", nil
	return reader, "application/octet-stream", nil
}

func (bodyCreator) pprofMultipart(pi *parser.PutInput) (bodyReader io.Reader, contentType string, err error) {
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
