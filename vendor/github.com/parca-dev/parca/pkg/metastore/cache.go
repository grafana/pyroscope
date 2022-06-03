// Copyright 2021 The Parca Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metastore

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/protobuf/proto"

	pb "github.com/parca-dev/parca/gen/proto/go/parca/metastore/v1alpha1"
)

type metaStoreCache struct {
	metrics *metrics

	locationsMtx   *sync.RWMutex
	locationsByID  map[string]*pb.Location
	locationsByKey map[LocationKey][]byte

	mappingsMtx   *sync.RWMutex
	mappingsByID  map[string]*pb.Mapping
	mappingsByKey map[MappingKey][]byte

	functionsMtx   *sync.RWMutex
	functionsByID  map[string]*pb.Function
	functionsByKey map[FunctionKey][]byte

	locationLinesMtx  *sync.RWMutex
	locationLinesByID map[string][]*pb.Line
}

type metrics struct {
	locationIDHits    prometheus.Counter
	locationIDMisses  prometheus.Counter
	locationKeyHits   prometheus.Counter
	locationKeyMisses prometheus.Counter

	mappingIDHits    prometheus.Counter
	mappingIDMisses  prometheus.Counter
	mappingKeyHits   prometheus.Counter
	mappingKeyMisses prometheus.Counter

	functionIDHits    prometheus.Counter
	functionIDMisses  prometheus.Counter
	functionKeyHits   prometheus.Counter
	functionKeyMisses prometheus.Counter

	locationLinesIDHits   prometheus.Counter
	locationLinesIDMisses prometheus.Counter
}

func newMetaStoreCacheMetrics(reg prometheus.Registerer) *metrics {
	idHits := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "parca_metastore_cache_id_hits_total",
			Help: "Number of cache hits for id lookups.",
		},
		[]string{"item_type"},
	)
	idMisses := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "parca_metastore_cache_id_misses_total",
			Help: "Number of cache misses for id lookups.",
		},
		[]string{"item_type"},
	)
	keyHits := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "parca_metastore_cache_key_hits_total",
			Help: "Number of cache hits for key lookups.",
		},
		[]string{"item_type"},
	)
	keyMisses := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "parca_metastore_cache_key_misses_total",
			Help: "Number of cache misses for key lookups.",
		},
		[]string{"item_type"},
	)

	m := &metrics{
		locationIDHits:    idHits.WithLabelValues("location"),
		locationIDMisses:  idMisses.WithLabelValues("location"),
		locationKeyHits:   keyHits.WithLabelValues("location"),
		locationKeyMisses: keyMisses.WithLabelValues("location"),

		mappingIDHits:    idHits.WithLabelValues("mapping"),
		mappingIDMisses:  idMisses.WithLabelValues("mapping"),
		mappingKeyHits:   keyHits.WithLabelValues("mapping"),
		mappingKeyMisses: keyMisses.WithLabelValues("mapping"),

		functionIDHits:    idHits.WithLabelValues("function"),
		functionIDMisses:  idMisses.WithLabelValues("function"),
		functionKeyHits:   keyHits.WithLabelValues("function"),
		functionKeyMisses: keyMisses.WithLabelValues("function"),

		locationLinesIDHits:   idHits.WithLabelValues("location_lines"),
		locationLinesIDMisses: idMisses.WithLabelValues("location_lines"),
	}

	if reg != nil {
		reg.MustRegister(idHits)
		reg.MustRegister(idMisses)
		reg.MustRegister(keyHits)
		reg.MustRegister(keyMisses)
	}

	return m
}

func newMetaStoreCache(reg prometheus.Registerer) *metaStoreCache {
	return &metaStoreCache{
		metrics: newMetaStoreCacheMetrics(reg),

		locationsMtx:   &sync.RWMutex{},
		locationsByID:  map[string]*pb.Location{},
		locationsByKey: map[LocationKey][]byte{},

		mappingsMtx:   &sync.RWMutex{},
		mappingsByID:  map[string]*pb.Mapping{},
		mappingsByKey: map[MappingKey][]byte{},

		functionsMtx:   &sync.RWMutex{},
		functionsByID:  map[string]*pb.Function{},
		functionsByKey: map[FunctionKey][]byte{},

		locationLinesMtx:  &sync.RWMutex{},
		locationLinesByID: map[string][]*pb.Line{},
	}
}

func (c *metaStoreCache) getLocationByKey(ctx context.Context, k LocationKey) (*pb.Location, bool, error) {
	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	default:
	}

	c.locationsMtx.RLock()
	defer c.locationsMtx.RUnlock()

	id, found := c.locationsByKey[k]
	if !found {
		c.metrics.locationKeyMisses.Inc()
		return nil, false, nil
	}

	l, found := c.locationsByID[string(id)]
	if !found {
		c.metrics.locationKeyMisses.Inc()
		return nil, false, nil
	}

	c.metrics.locationKeyHits.Inc()
	return proto.Clone(l).(*pb.Location), found, nil
}

func (c *metaStoreCache) getLocationByID(ctx context.Context, id []byte) (*pb.Location, bool, error) {
	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	default:
	}

	c.locationsMtx.RLock()
	defer c.locationsMtx.RUnlock()

	l, found := c.locationsByID[string(id)]
	if !found {
		c.metrics.locationIDHits.Inc()
		return nil, false, nil
	}

	c.metrics.locationIDHits.Inc()
	return proto.Clone(l).(*pb.Location), found, nil
}

func (c *metaStoreCache) setLocationByKey(ctx context.Context, k LocationKey, l *pb.Location) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.locationsMtx.Lock()
	defer c.locationsMtx.Unlock()

	c.locationsByID[string(l.Id)] = l
	c.locationsByKey[k] = l.Id

	return nil
}

func (c *metaStoreCache) setLocationByID(ctx context.Context, l *pb.Location) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.locationsMtx.Lock()
	defer c.locationsMtx.Unlock()

	c.locationsByID[string(l.Id)] = l

	return nil
}

func (c *metaStoreCache) getMappingByKey(ctx context.Context, k MappingKey) (*pb.Mapping, bool, error) {
	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	default:
	}

	c.mappingsMtx.RLock()
	defer c.mappingsMtx.RUnlock()

	id, found := c.mappingsByKey[k]
	if !found {
		c.metrics.mappingKeyMisses.Inc()
		return nil, false, nil
	}

	m, found := c.mappingsByID[string(id)]
	if !found {
		c.metrics.mappingKeyMisses.Inc()
		return nil, false, nil
	}

	c.metrics.mappingKeyHits.Inc()
	return proto.Clone(m).(*pb.Mapping), found, nil
}

func (c *metaStoreCache) getMappingByID(ctx context.Context, id []byte) (*pb.Mapping, bool, error) {
	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	default:
	}

	c.mappingsMtx.RLock()
	defer c.mappingsMtx.RUnlock()

	m, found := c.mappingsByID[string(id)]
	if !found {
		c.metrics.mappingIDHits.Inc()
		return nil, false, nil
	}

	c.metrics.mappingIDHits.Inc()
	return proto.Clone(m).(*pb.Mapping), found, nil
}

func (c *metaStoreCache) setMappingByKey(ctx context.Context, k MappingKey, m *pb.Mapping) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.mappingsMtx.Lock()
	defer c.mappingsMtx.Unlock()

	c.mappingsByID[string(m.Id)] = m
	c.mappingsByKey[k] = m.Id

	return nil
}

func (c *metaStoreCache) setMappingByID(ctx context.Context, m *pb.Mapping) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.mappingsMtx.Lock()
	defer c.mappingsMtx.Unlock()

	c.mappingsByID[string(m.Id)] = m

	return nil
}

func (c *metaStoreCache) getFunctionByKey(ctx context.Context, k FunctionKey) (*pb.Function, bool, error) {
	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	default:
	}

	c.functionsMtx.RLock()
	defer c.functionsMtx.RUnlock()

	id, found := c.functionsByKey[k]
	if !found {
		c.metrics.functionKeyMisses.Inc()
		return nil, false, nil
	}

	fn, found := c.functionsByID[string(id)]
	if !found {
		c.metrics.functionKeyMisses.Inc()
		return nil, false, nil
	}

	c.metrics.functionKeyHits.Inc()
	return proto.Clone(fn).(*pb.Function), found, nil
}

func (c *metaStoreCache) setFunctionByKey(ctx context.Context, k FunctionKey, f *pb.Function) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.functionsMtx.Lock()
	defer c.functionsMtx.Unlock()

	c.functionsByID[string(f.Id)] = f
	c.functionsByKey[k] = f.Id

	return nil
}

func (c *metaStoreCache) getFunctionByID(ctx context.Context, functionID []byte) (*pb.Function, bool, error) {
	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	default:
	}

	c.functionsMtx.RLock()
	defer c.functionsMtx.RUnlock()

	f, found := c.functionsByID[string(functionID)]
	if !found {
		c.metrics.functionIDMisses.Inc()
		return nil, false, nil
	}

	c.metrics.functionIDHits.Inc()
	return proto.Clone(f).(*pb.Function), found, nil
}

func (c *metaStoreCache) setFunctionByID(ctx context.Context, f *pb.Function) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.functionsMtx.Lock()
	defer c.functionsMtx.Unlock()

	c.functionsByID[string(f.Id)] = f
	return nil
}

func (c *metaStoreCache) setLocationLinesByID(ctx context.Context, locationID []byte, ll []*pb.Line) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	v := make([]*pb.Line, len(ll))
	for i, l := range ll {
		v[i] = proto.Clone(l).(*pb.Line)
	}

	c.locationLinesMtx.Lock()
	defer c.locationLinesMtx.Unlock()

	c.locationLinesByID[string(locationID)] = v

	return nil
}

func (c *metaStoreCache) getLocationLinesByID(ctx context.Context, locationID []byte) ([]*pb.Line, bool, error) {
	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	default:
	}

	c.locationLinesMtx.RLock()
	defer c.locationLinesMtx.RUnlock()

	ll, found := c.locationLinesByID[string(locationID)]
	if !found {
		c.metrics.locationLinesIDMisses.Inc()
		return nil, false, nil
	}

	v := make([]*pb.Line, len(ll))
	for i, l := range ll {
		v[i] = proto.Clone(l).(*pb.Line)
	}

	c.metrics.locationLinesIDHits.Inc()
	return v, true, nil
}
