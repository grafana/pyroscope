package remote

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/pyroscope-io/pyroscope/pkg/agent/log"
	"io"
	"net/http"
	"net/url"
	"path"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
)

var (
	ErrCloudTokenRequired = errors.New("Please provide an authentication token. You can find it here: https://pyroscope.io/cloud")
	cloudHostnameSuffix   = "pyroscope.cloud"
)

type Remote struct {
	cfg    RemoteConfig
	jobs   chan *upstream.UploadJob
	client *http.Client
	Logger log.Logger

	done chan struct{}
	wg   sync.WaitGroup
}

type RemoteConfig struct {
	AuthToken              string
	BasicAuthUser          string
	BasicAuthPassword      string
	ScopeOrgID             string
	HTTPHeaders            map[string]string
	UpstreamThreads        int
	UpstreamAddress        string
	UpstreamRequestTimeout time.Duration
}

func New(cfg RemoteConfig, logger log.Logger) (*Remote, error) {
	remote := &Remote{
		cfg:  cfg,
		jobs: make(chan *upstream.UploadJob, 100),
		client: &http.Client{
			Transport: &http.Transport{
				MaxConnsPerHost: cfg.UpstreamThreads,
			},
			Timeout: cfg.UpstreamRequestTimeout,
		},
		Logger: logger,
		done:   make(chan struct{}),
	}

	// parse the upstream address
	u, err := url.Parse(cfg.UpstreamAddress)
	if err != nil {
		return nil, err
	}

	// authorize the token first
	if cfg.AuthToken == "" && requiresAuthToken(u) {
		return nil, ErrCloudTokenRequired
	}

	return remote, nil
}

func (r *Remote) Start() {
	for i := 0; i < r.cfg.UpstreamThreads; i++ {
		go r.handleJobs()
	}
}

func (r *Remote) Stop() {
	if r.done != nil {
		close(r.done)
	}

	// wait for uploading goroutines exit
	r.wg.Wait()
}

func (r *Remote) Upload(job *upstream.UploadJob) {
	select {
	case r.jobs <- job:
	default:
		r.Logger.Errorf("remote upload queue is full, dropping a profile job")
	}
}

// UploadSync is only used in benchmarks right now
func (r *Remote) UploadSync(job *upstream.UploadJob) error {
	return r.uploadProfile(job)
}

func (r *Remote) uploadProfile(j *upstream.UploadJob) error {
	u, err := url.Parse(r.cfg.UpstreamAddress)
	if err != nil {
		return fmt.Errorf("url parse: %v", err)
	}

	q := u.Query()
	q.Set("name", j.Name)
	// TODO: I think these should be renamed to startTime / endTime
	q.Set("from", strconv.Itoa(int(j.StartTime.Unix())))
	q.Set("until", strconv.Itoa(int(j.EndTime.Unix())))
	q.Set("spyName", j.SpyName)
	q.Set("sampleRate", strconv.Itoa(int(j.SampleRate)))
	q.Set("units", string(j.Units))
	q.Set("aggregationType", string(j.AggregationType))

	u.Path = path.Join(u.Path, "ingest")
	u.RawQuery = q.Encode()

	r.Logger.Debugf("uploading at %s", u.String())
	// new a request for the job
	request, err := http.NewRequest("POST", u.String(), bytes.NewReader(j.Trie.Bytes()))
	if err != nil {
		return fmt.Errorf("new http request: %v", err)
	}
	request.Header.Set("Content-Type", "binary/octet-stream+trie")

	if r.cfg.AuthToken != "" {
		request.Header.Set("Authorization", "Bearer "+r.cfg.AuthToken)
	} else if r.cfg.BasicAuthUser != "" && r.cfg.BasicAuthPassword != "" {
		request.SetBasicAuth(r.cfg.BasicAuthUser, r.cfg.BasicAuthPassword)
	}
	if r.cfg.ScopeOrgID != "" {
		request.Header.Set("X-Scope-OrgID", r.cfg.ScopeOrgID)
	}
	for k, v := range r.cfg.HTTPHeaders {
		request.Header.Set(k, v)
	}

	// do the request and get the response
	response, err := r.client.Do(request)
	if err != nil {
		return fmt.Errorf("do http request: %v", err)
	}
	defer response.Body.Close()

	// read all the response body
	respBody, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read response body: %v", err)
	}

	if response.StatusCode != 200 {
		return fmt.Errorf("failed to upload. server responded with statusCode: '%d' and body: '%s'", response.StatusCode, string(respBody))
	}

	return nil
}

// handle the jobs
func (r *Remote) handleJobs() {
	for {
		select {
		case <-r.done:
			return
		case job := <-r.jobs:
			r.safeUpload(job)
		}
	}
}

func requiresAuthToken(u *url.URL) bool {
	return strings.HasSuffix(u.Host, cloudHostnameSuffix)
}

// do safe upload
func (r *Remote) safeUpload(job *upstream.UploadJob) {
	defer func() {
		if catch := recover(); catch != nil {
			r.Logger.Errorf("recover stack: %v", debug.Stack())
		}
	}()

	// update the profile data to server
	if err := r.uploadProfile(job); err != nil {
		r.Logger.Errorf("upload profile: %v", err)
	}
}
