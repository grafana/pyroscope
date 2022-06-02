package remotewrite

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/parser"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/sirupsen/logrus"
)

var (
	ErrConvertPutInputToRequest = errors.New("failed to convert putInput into a http.Request")
	ErrMakingRequest            = errors.New("failed to make request")
	ErrNotOkResponse            = errors.New("response not ok")
)

type Client struct {
	log    *logrus.Logger
	config config.RemoteWrite
	client *http.Client
}

func NewClient(logger *logrus.Logger, cfg config.RemoteWrite) *Client {
	client := &http.Client{
		// TODO(eh-am): make timeout configurable
		Timeout: time.Second * 15,
	}

	return &Client{
		log:    logger,
		config: cfg,
		client: client,
	}
}

func (r *Client) Put(ctx context.Context, put *parser.PutInput) error {
	req, err := r.putInputToRequest(put)
	if err != nil {
		return multierror.Append(err, ErrConvertPutInputToRequest)
	}

	r.enhanceWithAuth(req)

	req = req.WithContext(ctx)
	r.log.Debugf("Making request to %s", req.URL.String())
	res, err := r.client.Do(req)
	if err != nil {
		return multierror.Append(err, ErrMakingRequest)
	}

	if !(res.StatusCode >= 200 && res.StatusCode < 300) {
		return multierror.Append(ErrNotOkResponse, fmt.Errorf("status code: '%d'", res.StatusCode))
	}

	return nil
}

func streamToByte(stream io.Reader) []byte {
	buf := new(bytes.Buffer)
	buf.ReadFrom(stream)
	return buf.Bytes()
}

func (r *Client) putInputToRequest(pi *parser.PutInput) (*http.Request, error) {
	pi = pi.Clone()

	// TODO(eh-am): is this the most efficient to do this?
	// maybe we shouldn't even clone it
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fw, err := writer.CreateFormFile("profile", "profile.pprof")
	fw.Write(streamToByte(pi.Profile))
	if err != nil {
		return nil, err
	}

	if pi.PreviousProfile != nil {
		fw, err = writer.CreateFormFile("prev_profile", "profile.pprof")
		fw.Write(streamToByte(pi.PreviousProfile))
		if err != nil {
			return nil, err
		}
	}
	writer.Close()

	req, err := http.NewRequest("POST", r.config.Address, body)
	if err != nil {
		return nil, err
	}

	r.enhanceWithTags(pi.Key)

	params := req.URL.Query()
	params.Set("name", pi.Key.Normalized())
	params.Set("from", strconv.FormatInt(pi.StartTime.Unix(), 10))
	params.Set("until", strconv.FormatInt(pi.EndTime.Unix(), 10))
	params.Set("sampleRate", strconv.FormatUint(uint64(pi.SampleRate), 10))
	params.Set("spyName", pi.SpyName)
	params.Set("units", pi.Units.String())
	params.Set("aggregationType", pi.AggregationType.String())

	req.URL.RawQuery = params.Encode()

	contentType := writer.FormDataContentType()
	req.Header.Set("Content-Type", contentType)

	return req, nil
}

func (r *Client) enhanceWithTags(key *segment.Key) {
	labels := key.Labels()
	for tag, value := range r.config.Tags {
		labels[tag] = value
	}
}

func (r *Client) enhanceWithAuth(req *http.Request) {
	token := r.config.AuthToken

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}
