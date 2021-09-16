package direct

import (
	"runtime/debug"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/profile"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
)

const upstreamThreads = 1

type Direct struct {
	storage *storage.Storage
	queue   chan *upstream.UploadJob
	stop    chan struct{}
	wg      sync.WaitGroup
}

func New(s *storage.Storage) *Direct {
	return &Direct{
		storage: s,
		queue:   make(chan *upstream.UploadJob, 100),
		stop:    make(chan struct{}),
	}
}

func (u *Direct) Start() {
	u.wg.Add(upstreamThreads)
	for i := 0; i < upstreamThreads; i++ {
		go u.uploadLoop()
	}
}

func (u *Direct) Stop() {
	close(u.stop)
	u.wg.Wait()
}

func (u *Direct) Upload(j *upstream.UploadJob) {
	select {
	case u.queue <- j:
	case <-u.stop:
		return
	default:
		logrus.Error("Direct upload queue is full, dropping a profile")
	}
}

func (u *Direct) uploadProfile(j *upstream.UploadJob) {
	key, err := segment.ParseKey(j.Name)
	if err != nil {
		logrus.WithField("key", key).Error("invalid key:")
		return
	}

	sym := u.storage.Symbols(key.AppName())
	p := profile.New()
	var s uint64
	sym.Lock()
	j.Trie.Iterate(func(name []byte, val uint64) {
		sym.Insert(p, name, val)
		s += val
	})
	sym.Unlock()

	pi := &storage.PutInput{
		StartTime:       j.StartTime,
		EndTime:         j.EndTime,
		Key:             key,
		Samples:         s,
		Val:             p,
		SpyName:         j.SpyName,
		SampleRate:      j.SampleRate,
		Units:           j.Units,
		AggregationType: j.AggregationType,
	}
	if err = u.storage.Put(pi); err != nil {
		logrus.WithError(err).Error("failed to store a local profile")
	}
}

func (u *Direct) uploadLoop() {
	defer u.wg.Done()
	for {
		select {
		case j := <-u.queue:
			u.safeUpload(j)
		case <-u.stop:
			return
		}
	}
}

// do safe upload
func (u *Direct) safeUpload(j *upstream.UploadJob) {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("panic recovered: %v; %v", r, string(debug.Stack()))
		}
	}()
	u.uploadProfile(j)
}
