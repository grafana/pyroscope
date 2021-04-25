package direct

import (
	"runtime/debug"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

const upstreamThreads = 1

type Direct struct {
	cfg  *config.Config
	s    *storage.Storage
	todo chan *upstream.UploadJob
	done chan struct{}
}

func New(cfg *config.Config, s *storage.Storage) *Direct {
	d := &Direct{
		cfg:  cfg,
		s:    s,
		todo: make(chan *upstream.UploadJob, 100),
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

func (u *Direct) Upload(j *upstream.UploadJob) {
	select {
	case u.todo <- j:
	default:
		logrus.Error("Direct upload queue is full, dropping a profile")
	}
}

func (u *Direct) uploadProfile(j *upstream.UploadJob) {
	key, err := storage.ParseKey(j.Name)
	if err != nil {
		logrus.WithField("key", key).Error("invalid key:")
		return
	}

	t := tree.New()
	j.Trie.Iterate(func(name []byte, val uint64) {
		t.Insert(name, val, false)
	})

	u.s.Put(&storage.PutInput{
		StartTime:       j.StartTime,
		EndTime:         j.EndTime,
		Key:             key,
		Val:             t,
		SpyName:         j.SpyName,
		SampleRate:      j.SampleRate,
		Units:           j.Units,
		AggregationType: j.AggregationType,
	})
}

func (u *Direct) uploadLoop() {
	for {
		select {
		case j := <-u.todo:
			u.safeUpload(j)
		case <-u.done:
			return
		}
	}
}

// do safe upload
func (u *Direct) safeUpload(j *upstream.UploadJob) {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("panic, stack = : %v", debug.Stack())
		}
	}()

	u.uploadProfile(j)
}
