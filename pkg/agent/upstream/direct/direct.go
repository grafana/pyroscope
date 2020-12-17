package direct

import (
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
)

type uploadJob struct {
	name      string
	startTime time.Time
	endTime   time.Time
	t         *transporttrie.Trie
}

type Direct struct {
	cfg    *config.Config
	s      *storage.Storage
	todo   chan *uploadJob
	done   chan struct{}
	client *http.Client
}

func New(cfg *config.Config, s *storage.Storage) *Direct {
	return &Direct{
		cfg:  cfg,
		s:    s,
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

func (u *Direct) Start() {
	for i := 0; i < u.cfg.Agent.UpstreamThreads; i++ {
		go u.uploadLoop()
	}
}

func (u *Direct) Stop() {
	for i := 0; i < u.cfg.Agent.UpstreamThreads; i++ {
		u.done <- struct{}{}
	}
}

// TODO: this metadata class should be unified
func (u *Direct) Upload(name string, startTime time.Time, endTime time.Time, t *transporttrie.Trie) {
	job := &uploadJob{
		name:      name,
		startTime: startTime,
		endTime:   endTime,
		t:         t,
	}
	select {
	case u.todo <- job:
	default:
		log.Error("Direct upload queue is full, dropping a profile")
	}
}

func (u *Direct) uploadProfile(j *uploadJob) {
	key, err := storage.ParseKey(j.name)
	if err != nil {
		logrus.WithField("key", key).Error("invalid key:")
		return
	}

	t := tree.New()
	j.t.Iterate(func(name []byte, val uint64) {
		t.Insert(name, val, false)
	})

	u.s.Put(j.startTime, j.endTime, key, t)
}

func (u *Direct) uploadLoop() {
	for {
		select {
		case j := <-u.todo:
			u.uploadProfile(j)
		case <-u.done:
			return
		}
	}
}
