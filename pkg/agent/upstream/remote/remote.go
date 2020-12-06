package remote

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/petethepig/pyroscope/pkg/config"
	"github.com/petethepig/pyroscope/pkg/structs/transporttrie"
)

type uploadJob struct {
	name      string
	startTime time.Time
	endTime   time.Time
	t         *transporttrie.Trie
}

type Remote struct {
	cfg    *config.Config
	todo   chan *uploadJob
	done   chan struct{}
	client *http.Client
}

func New(cfg *config.Config) *Remote {
	return &Remote{
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

func (u *Remote) Start() {
	for i := 0; i < u.cfg.Agent.UpstreamThreads; i++ {
		go u.uploadLoop()
	}
}

func (u *Remote) Stop() {
	for i := 0; i < u.cfg.Agent.UpstreamThreads; i++ {
		u.done <- struct{}{}
	}
}

// TODO: this metadata class should be unified
func (u *Remote) Upload(name string, startTime time.Time, endTime time.Time, t *transporttrie.Trie) {
	job := &uploadJob{
		name:      name,
		startTime: startTime,
		endTime:   endTime,
		t:         t,
	}
	select {
	case u.todo <- job:
	default:
		log.Error("Remote upload queue is full, dropping a profile")
	}
}

func (u *Remote) uploadProfile(j *uploadJob) {
	urlObj, _ := url.Parse(u.cfg.Agent.UpstreamAddress)
	q := urlObj.Query()

	q.Set("name", j.name)
	// TODO: I think these should be renamed in favor of startTime endTime
	q.Set("from", strconv.Itoa(int(j.startTime.Unix())))
	q.Set("until", strconv.Itoa(int(j.endTime.Unix())))

	urlObj.Path = "/ingest"
	urlObj.RawQuery = q.Encode()
	buf := j.t.Bytes()
	log.Info("uploading at ", urlObj.String())
	resp, err := u.client.Post(urlObj.String(), "binary/octet-stream+trie", bytes.NewReader(buf))
	if err != nil {
		log.Error("Error happened when uploading a profile:", err)
	}
	if resp != nil {
		_, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Error("Error happened while reading server response:", err)
		}
	}
}

func (u *Remote) uploadLoop() {
	for {
		select {
		case j := <-u.todo:
			u.uploadProfile(j)
		case <-u.done:
			return
		}
	}
}
