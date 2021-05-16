package remote

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
)

var (
	ErrCloudTokenRequired = errors.New("Please provide an authentication token. You can find it here: https://pyroscope.io/cloud")
	cloudHostnameSuffix   = "pyroscope.cloud"
)

type Remote struct {
	cfg    RemoteConfig
	todo   chan *upstream.UploadJob
	done   chan *sync.WaitGroup
	client *http.Client

	Logger agent.Logger
}

type RemoteConfig struct {
	AuthToken              string
	UpstreamThreads        int
	UpstreamAddress        string
	UpstreamRequestTimeout time.Duration
}

func New(cfg RemoteConfig) (*Remote, error) {
	r := &Remote{
		cfg:  cfg,
		todo: make(chan *upstream.UploadJob, 100),
		done: make(chan *sync.WaitGroup, cfg.UpstreamThreads),
		client: &http.Client{
			Transport: &http.Transport{
				MaxConnsPerHost: cfg.UpstreamThreads,
			},
			Timeout: cfg.UpstreamRequestTimeout,
		},
	}

	urlObj, err := url.Parse(cfg.UpstreamAddress)
	if err != nil {
		return nil, err
	}

	if cfg.AuthToken == "" && requiresAuthToken(urlObj) {
		return nil, ErrCloudTokenRequired
	}

	go r.start()
	return r, nil
}

func (u *Remote) start() {
	for i := 0; i < u.cfg.UpstreamThreads; i++ {
		go u.uploadLoop()
	}
}

func (u *Remote) Stop() {
	wg := sync.WaitGroup{}
	wg.Add(u.cfg.UpstreamThreads)
	for i := 0; i < u.cfg.UpstreamThreads; i++ {
		u.done <- &wg
	}
	wg.Wait()
}

// TODO: this metadata class should be unified
func (u *Remote) Upload(j *upstream.UploadJob) {
	select {
	case u.todo <- j:
	default:
		if u.Logger != nil {
			u.Logger.Errorf("Remote upload queue is full, dropping a profile")
		}
	}
}

func (u *Remote) uploadProfile(j *upstream.UploadJob) {
	urlObj, _ := url.Parse(u.cfg.UpstreamAddress)
	q := urlObj.Query()

	q.Set("name", j.Name)
	// TODO: I think these should be renamed to startTime / endTime
	q.Set("from", strconv.Itoa(int(j.StartTime.Unix())))
	q.Set("until", strconv.Itoa(int(j.EndTime.Unix())))
	q.Set("spyName", j.SpyName)
	q.Set("sampleRate", strconv.Itoa(int(j.SampleRate)))
	q.Set("units", j.Units)
	q.Set("aggregationType", j.AggregationType)

	urlObj.Path = path.Join(urlObj.Path, "/ingest")
	urlObj.RawQuery = q.Encode()
	buf := j.Trie.Bytes()
	if u.Logger != nil {
		u.Logger.Infof("uploading at %s", urlObj.String())
	}

	req, err := http.NewRequest("POST", urlObj.String(), bytes.NewReader(buf))
	if err != nil {
		if u.Logger != nil {
			u.Logger.Errorf("Error happened when uploading a profile: %v", err)
		}
		return
	}
	req.Header.Set("Content-Type", "binary/octet-stream+trie")
	if u.cfg.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+u.cfg.AuthToken)
	}
	resp, err := u.client.Do(req)
	if err != nil {
		if u.Logger != nil {
			u.Logger.Errorf("Error happened when uploading a profile: %v", err)
		}
		return
	}

	if resp != nil {
		defer resp.Body.Close()
		_, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			if u.Logger != nil {
				u.Logger.Errorf("Error happened while reading server response: %v", err)
			}
		}
	}
}

func (u *Remote) uploadLoop() {
	for {
		select {
		case j := <-u.todo:
			u.safeUpload(j)
		case wg := <-u.done:
			wg.Done()
			return
		}
	}
}

func requiresAuthToken(u *url.URL) bool {
	return strings.HasSuffix(u.Host, cloudHostnameSuffix)
}

// do safe upload
func (u *Remote) safeUpload(j *upstream.UploadJob) {
	defer func() {
		if r := recover(); r != nil {
			if u.Logger != nil {
				u.Logger.Errorf("panic, stack = : %v", debug.Stack())
			}
		}
	}()

	u.uploadProfile(j)
}
