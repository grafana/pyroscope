// Copyright 2023-2025 The Parca Authors
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

package lru

import (
	"errors"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type metrics struct {
	hits, misses, evictions prometheus.Counter

	unregisterer func() error
}

func newMetrics(reg prometheus.Registerer) *metrics {
	requests := promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
		Name: "cache_requests_total",
		Help: "Total number of cache requests.",
	}, []string{"result"})
	evictions := promauto.With(reg).NewCounter(prometheus.CounterOpts{
		Name: "cache_evictions_total",
		Help: "Total number of cache evictions.",
	})
	return &metrics{
		hits:      requests.WithLabelValues("hit"),
		misses:    requests.WithLabelValues("miss"),
		evictions: evictions,

		unregisterer: func() error {
			// This closer makes sure that the metrics are unregistered when the cache is closed.
			// This is useful when the a new cache is created with the same name.
			var err error
			if ok := reg.Unregister(requests); !ok {
				err = errors.Join(err, fmt.Errorf("unregistering requests counter: %w", err))
			}
			if ok := reg.Unregister(evictions); !ok {
				err = errors.Join(err, fmt.Errorf("unregistering eviction counter: %w", err))
			}
			if err != nil {
				return fmt.Errorf("cleaning cache stats counter: %w", err)
			}
			return nil
		},
	}
}

func (m *metrics) unregister() error {
	return m.unregisterer()
}
