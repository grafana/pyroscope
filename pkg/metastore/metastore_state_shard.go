package metastore

import (
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

func newMetastoreShard() *metastoreShard {
	return &metastoreShard{
		segments: make(map[string]*metastorev1.BlockMeta),
	}
}

func (s *metastoreShard) put(segment *metastorev1.BlockMeta) {
	s.segmentsMutex.Lock()
	s.segments[segment.Id] = segment
	s.segmentsMutex.Unlock()
}
