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

package cache

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

func TestLoadingOnceCache(t *testing.T) {
	var counter atomic.Uint32
	loader := func(key string) (string, error) {
		counter.Add(1)
		return "value", nil
	}
	c := NewLoadingOnceCache(prometheus.NewRegistry(), 128, time.Second, loader)

	// First call loads value.
	go func() {
		for i := 0; i < 10; i++ {
			go func() {
				v, err := c.Get("key")
				require.NoError(t, err)
				require.Equal(t, "value", v)
			}()
		}
	}()
	v, err := c.Get("key")
	require.NoError(t, err)
	require.Equal(t, "value", v)

	require.Equal(t, uint32(1), counter.Load())
}
