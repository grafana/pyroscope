package remotewrite

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
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
	log         *logrus.Logger
	config      config.RemoteWrite
	client      *http.Client
	bodyCreator *BodyCreator
}

func NewClient(logger *logrus.Logger, cfg config.RemoteWrite) *Client {
	client := &http.Client{
		// TODO(eh-am): make timeout configurable
		Timeout: time.Second * 15,
	}

	return &Client{
		log:         logger,
		config:      cfg,
		client:      client,
		bodyCreator: NewBodyCreator(logger),
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
		// read all the response body
		respBody, _ := ioutil.ReadAll(res.Body)
		return multierror.Append(ErrNotOkResponse, fmt.Errorf("status code: '%d'. body: '%s'", res.StatusCode, respBody))
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

	body, contentType, err := r.bodyCreator.Create(pi)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", r.config.Address+"/ingest", body)
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
