package upstream

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/url"

	log "github.com/sirupsen/logrus"

	"github.com/petethepig/pyroscope/pkg/config"
	"github.com/petethepig/pyroscope/pkg/structs/transporttrie"
)

type uploadJob struct {
	m map[string]string
	t *transporttrie.Trie
}

type Upstream struct {
	cfg    *config.Config
	todo   chan *uploadJob
	done   chan struct{}
	client *http.Client
}

func New(cfg *config.Config) *Upstream {
	return &Upstream{
		cfg:  cfg,
		todo: make(chan *uploadJob, 100),
		done: make(chan struct{}, cfg.Agent.UpstreamThreads),
		client: &http.Client{
			Transport: &http.Transport{
				MaxConnsPerHost: cfg.Agent.UpstreamThreads,
			},
			Timeout: cfg.Agent.UpstreamRequestTimeout,
		},
	}
}

func (u *Upstream) Start() {
	for i := 0; i < u.cfg.Agent.UpstreamThreads; i++ {
		go u.uploadLoop()
	}
}

func (u *Upstream) Stop() {
	for i := 0; i < u.cfg.Agent.UpstreamThreads; i++ {
		u.done <- struct{}{}
	}
}

// TODO: this metadata class should be unified
func (u *Upstream) Upload(metadata map[string]string, t *transporttrie.Trie) {
	job := &uploadJob{
		m: metadata,
		t: t,
	}
	select {
	case u.todo <- job:
	default:
		log.Error("Upstream upload queue is full, dropping a profile")
	}
}

func (u *Upstream) uploadProfile(j *uploadJob) {
	urlObj, _ := url.Parse(u.cfg.Agent.UpstreamAddress)
	q := urlObj.Query()
	for k, v := range j.m {
		q.Set(k, v)
	}
	urlObj.Path = "/ingest"
	urlObj.RawQuery = q.Encode()
	buf := j.t.Bytes()
	resp, err := u.client.Post(urlObj.String(), "binary/octet-stream+trie", bytes.NewReader(buf))
	if err != nil {
		log.Error("Error happened when uploading a profile:", err)
	}
	if resp != nil {
		b, err2 := ioutil.ReadAll(resp.Body)
		log.Debug("log:", b, err, err2)
	}
}

func (u *Upstream) uploadLoop() {
	for {
		select {
		case j := <-u.todo:
			u.uploadProfile(j)
		case <-u.done:
			return
		}
	}
}
