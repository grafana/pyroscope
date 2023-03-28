package remotewrite

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
)

var (
	ErrConvertPutInputToRequest = errors.New("failed to convert putInput into a http.Request")
	ErrMakingRequest            = errors.New("failed to make request")
	ErrNotOkResponse            = errors.New("response not ok")
)

type Client struct {
	log     logrus.FieldLogger
	config  config.RemoteWriteTarget
	client  *http.Client
	metrics *clientMetrics
}

func NewClient(logger logrus.FieldLogger, reg prometheus.Registerer, targetName string, cfg config.RemoteWriteTarget) *Client {
	// setup defaults
	if cfg.Timeout == 0 {
		cfg.Timeout = time.Second * 10
	}

	client := &http.Client{
		Timeout: cfg.Timeout,
		Transport: &http.Transport{
			MaxConnsPerHost:     numWorkers(),
			MaxIdleConns:        numWorkers(),
			MaxIdleConnsPerHost: numWorkers(),
		},
	}

	metrics := newClientMetrics(reg, targetName, cfg.Address)
	metrics.mustRegister()

	return &Client{
		log:     logger,
		config:  cfg,
		client:  client,
		metrics: metrics,
	}
}

func (r *Client) Ingest(ctx context.Context, in *ingestion.IngestInput) error {
	req, err := r.ingestInputToRequest(in)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrConvertPutInputToRequest, err)
	}

	r.enhanceWithAuth(req)

	req = req.WithContext(ctx)

	b, err := in.Profile.Bytes()
	if err == nil {
		// TODO(petethepig): we might want to improve accuracy of this metric at some point
		//   see comment here: https://github.com/pyroscope-io/pyroscope/pull/1147#discussion_r894975126
		r.metrics.sentBytes.Add(float64(len(b)))
	}

	start := time.Now()
	res, err := r.client.Do(req)
	if res != nil {
		// sometimes both res and err are non-nil
		// therefore we must always close the body first
		defer res.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("%w: %v", ErrMakingRequest, err)
	}

	duration := time.Since(start)
	r.metrics.responseTime.With(prometheus.Labels{
		"code": strconv.FormatInt(int64(res.StatusCode), 10),
	}).Observe(duration.Seconds())

	if !(res.StatusCode >= 200 && res.StatusCode < 300) {
		// read all the response body
		respBody, _ := ioutil.ReadAll(res.Body)
		return fmt.Errorf("%w: %v", ErrNotOkResponse, fmt.Errorf("status code: '%d'. body: '%s'", res.StatusCode, respBody))
	}

	return nil
}

func (r *Client) ingestInputToRequest(in *ingestion.IngestInput) (*http.Request, error) {
	b, err := in.Profile.Bytes()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", r.config.Address+"/ingest", bytes.NewReader(b))
	for k, v := range r.config.Headers {
		req.Header.Set(k, v)
	}
	if err != nil {
		return nil, err
	}

	params := req.URL.Query()
	if in.Format != "" {
		params.Set("format", string(in.Format))
	}

	params.Set("name", r.getQuery(in.Metadata.Key))
	params.Set("from", strconv.FormatInt(in.Metadata.StartTime.Unix(), 10))
	params.Set("until", strconv.FormatInt(in.Metadata.EndTime.Unix(), 10))
	params.Set("sampleRate", strconv.FormatUint(uint64(in.Metadata.SampleRate), 10))
	params.Set("spyName", in.Metadata.SpyName)
	params.Set("units", in.Metadata.Units.String())
	params.Set("aggregationType", in.Metadata.AggregationType.String())
	req.URL.RawQuery = params.Encode()

	req.Header.Set("Content-Type", in.Profile.ContentType())

	return req, nil
}

func (r *Client) getQuery(key *segment.Key) string {
	k := key.Clone()

	labels := k.Labels()
	for tag, value := range r.config.Tags {
		labels[tag] = value
	}

	return k.Normalized()
}

func (r *Client) enhanceWithAuth(req *http.Request) {
	token := r.config.AuthToken

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}
