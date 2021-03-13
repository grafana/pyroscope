package direct

import (
	"time"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
)

const upstreamThreads = 1

type uploadJob struct {
	name       string
	startTime  time.Time
	endTime    time.Time
	t          *transporttrie.Trie
	spyName    string
	sampleRate int
}

type Direct struct {
	cfg  *config.Config
	s    *storage.Storage
	todo chan *uploadJob
	done chan struct{}
}

func New(cfg *config.Config, s *storage.Storage) *Direct {
	d := &Direct{
		cfg:  cfg,
		s:    s,
		todo: make(chan *uploadJob, 100),
		done: make(chan struct{}, upstreamThreads),
	}

	go d.start()
	return d
}

func (u *Direct) start() {
	for i := 0; i < upstreamThreads; i++ {
		go u.uploadLoop()
	}
}

func (u *Direct) Stop() {
	for i := 0; i < upstreamThreads; i++ {
		u.done <- struct{}{}
	}
}

// TODO: this metadata class should be unified
func (u *Direct) Upload(name string, startTime, endTime time.Time, spyName string, sampleRate int, t *transporttrie.Trie) {
	job := &uploadJob{
		name:       name,
		startTime:  startTime,
		endTime:    endTime,
		t:          t,
		spyName:    spyName,
		sampleRate: sampleRate,
	}
	select {
	case u.todo <- job:
	default:
		logrus.Error("Direct upload queue is full, dropping a profile")
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

	// TODO: pass spy name and sample rate from somewhere
	u.s.Put(j.startTime, j.endTime, key, t, j.spyName, j.sampleRate)
}

func (u *Direct) uploadLoop() {
	for {
		select {
		case j := <-u.todo:
			logrus.Debug("upload profile")
			u.uploadProfile(j)
		case <-u.done:
			return
		}
	}
}
