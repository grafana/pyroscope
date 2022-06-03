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
	default:
		return nil, "", ErrUnsupportedFormat
	}
}

func (BodyCreator) pprof(pi *parser.PutInput) (bodyReader io.Reader, contentType string, err error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fw, err := writer.CreateFormFile("profile", "profile.pprof")
	fw.Write(streamToByte(pi.Profile))
	if err != nil {
		return nil, "", err
	}

	if pi.PreviousProfile != nil {
		fw, err = writer.CreateFormFile("prev_profile", "profile.pprof")
		fw.Write(streamToByte(pi.PreviousProfile))
		if err != nil {
			return nil, "", err
		}
	}
	writer.Close()

	return body, writer.FormDataContentType(), nil
}
