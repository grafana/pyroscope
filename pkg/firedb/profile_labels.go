package firedb

import (
	"sync"
	"unsafe"

	"go.uber.org/atomic"

	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
)

type labelCache struct {
	labels map[labelKey]*profilev1.Label
	rw     sync.RWMutex
	size   atomic.Uint64
}

const labelSize = uint64(unsafe.Sizeof(profilev1.Label{}))

func (lc *labelCache) init() {
	lc.labels = make(map[labelKey]*profilev1.Label)
}

func (lc *labelCache) rewriteLabels(t stringConversionTable, in []*profilev1.Label) []*profilev1.Label {
	lc.rw.RLock()
	defer lc.rw.RUnlock()
	for i, l := range in {
		k := labelKey{
			Key:     t[l.Key],
			NumUnit: t[l.NumUnit],
			Str:     t[l.Str],
			Num:     l.Num,
		}
		l, ok := lc.labels[k]
		if ok {
			in[i] = l
			continue
		}
		lc.rw.RUnlock()
		lc.rw.Lock()
		l, ok = lc.labels[k]
		if !ok {
			l = &profilev1.Label{
				Key:     k.Key,
				NumUnit: k.NumUnit,
				Str:     k.Str,
				Num:     k.Num,
			}
			lc.size.Add(labelSize)
			lc.labels[k] = l
			in[i] = l
			lc.rw.Unlock()
			lc.rw.RLock()
			continue
		}
		lc.rw.Unlock()
		lc.rw.RLock()
		in[i] = l
	}
	return in
}

type labelKey struct {
	Key     int64
	Str     int64
	Num     int64
	NumUnit int64
}
