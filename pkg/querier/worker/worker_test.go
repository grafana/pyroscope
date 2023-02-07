// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/querier/worker/worker_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package worker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/flagext"
	"github.com/grafana/dskit/services"
	"github.com/grafana/dskit/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/grafana/phlare/pkg/scheduler/schedulerdiscovery"
	"github.com/grafana/phlare/pkg/util/servicediscovery"
)

func TestConfig_Validate(t *testing.T) {
	tests := map[string]struct {
		setup       func(cfg *Config)
		expectedErr string
	}{
		"should pass with default config": {
			setup: func(cfg *Config) {},
		},
		"should pass if scheduler address is configured": {
			setup: func(cfg *Config) {
				cfg.SchedulerAddress = "localhost:9095"
			},
		},
		"should pass if query-scheduler service discovery is set to ring, and no frontend and scheduler address is configured": {
			setup: func(cfg *Config) {
				cfg.QuerySchedulerDiscovery.Mode = schedulerdiscovery.ModeRing
			},
		},
		"should fail if query-scheduler service discovery is set to ring, and scheduler address is configured": {
			setup: func(cfg *Config) {
				cfg.QuerySchedulerDiscovery.Mode = schedulerdiscovery.ModeRing
				cfg.SchedulerAddress = "localhost:9095"
			},
			expectedErr: `scheduler address cannot be specified when query-scheduler service discovery mode is set to 'ring'`,
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			cfg := Config{}
			flagext.DefaultValues(&cfg)
			testData.setup(&cfg)

			actualErr := cfg.Validate(log.NewNopLogger())
			if testData.expectedErr == "" {
				require.NoError(t, actualErr)
			} else {
				require.Error(t, actualErr)
				assert.ErrorContains(t, actualErr, testData.expectedErr)
			}
		})
	}
}

func TestResetConcurrency(t *testing.T) {
	tests := []struct {
		name                string
		maxConcurrent       int
		numTargets          int
		numInUseTargets     int
		expectedConcurrency int
	}{
		{
			name:                "Create at least one processor per target if max concurrent = 0, with all targets in use",
			maxConcurrent:       0,
			numTargets:          2,
			numInUseTargets:     2,
			expectedConcurrency: 2,
		},
		{
			name:                "Create at least one processor per target if max concurrent = 0, with some targets in use",
			maxConcurrent:       0,
			numTargets:          2,
			numInUseTargets:     1,
			expectedConcurrency: 2,
		},
		{
			name:                "Max concurrent dividing with a remainder, with all targets in use",
			maxConcurrent:       7,
			numTargets:          4,
			numInUseTargets:     4,
			expectedConcurrency: 7,
		},
		{
			name:            "Max concurrent dividing with a remainder, with some targets in use",
			maxConcurrent:   7,
			numTargets:      4,
			numInUseTargets: 2,
			expectedConcurrency:/* in use:  */ 7 + /* not in use : */ 2,
		},
		{
			name:                "Max concurrent dividing evenly, with all targets in use",
			maxConcurrent:       6,
			numTargets:          2,
			numInUseTargets:     2,
			expectedConcurrency: 6,
		},
		{
			name:            "Max concurrent dividing evenly, with some targets in use",
			maxConcurrent:   6,
			numTargets:      4,
			numInUseTargets: 2,
			expectedConcurrency:/* in use:  */ 6 + /* not in use : */ 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				MaxConcurrentRequests: tt.maxConcurrent,
			}

			w, err := newQuerierWorkerWithProcessor(cfg, log.NewNopLogger(), &mockProcessor{}, nil, nil)
			require.NoError(t, err)
			require.NoError(t, services.StartAndAwaitRunning(context.Background(), w))

			for i := 0; i < tt.numTargets; i++ {
				// gRPC connections are virtual... they don't actually try to connect until they are needed.
				// This allows us to use dummy ports, and not get any errors.
				w.InstanceAdded(servicediscovery.Instance{
					Address: fmt.Sprintf("127.0.0.1:%d", i),
					InUse:   i < tt.numInUseTargets,
				})
			}

			test.Poll(t, 250*time.Millisecond, tt.expectedConcurrency, func() interface{} {
				return getConcurrentProcessors(w)
			})

			require.NoError(t, services.StopAndAwaitTerminated(context.Background(), w))
			assert.Equal(t, 0, getConcurrentProcessors(w))
		})
	}
}

func TestQuerierWorker_getDesiredConcurrency(t *testing.T) {
	tests := map[string]struct {
		instances     []servicediscovery.Instance
		maxConcurrent int
		expected      map[string]int
	}{
		"should return empty map on no instances": {
			instances:     nil,
			maxConcurrent: 4,
			expected:      map[string]int{},
		},
		"should divide the max concurrency between in-use instances, and create 1 connection for each instance not in-use": {
			instances: []servicediscovery.Instance{
				{Address: "1.1.1.1", InUse: true},
				{Address: "2.2.2.2", InUse: false},
				{Address: "3.3.3.3", InUse: true},
				{Address: "4.4.4.4", InUse: false},
			},
			maxConcurrent: 4,
			expected: map[string]int{
				"1.1.1.1": 2,
				"2.2.2.2": 1,
				"3.3.3.3": 2,
				"4.4.4.4": 1,
			},
		},
		"should create 1 connection for each instance if max concurrency is set to 0": {
			instances: []servicediscovery.Instance{
				{Address: "1.1.1.1", InUse: true},
				{Address: "2.2.2.2", InUse: false},
				{Address: "3.3.3.3", InUse: true},
				{Address: "4.4.4.4", InUse: false},
			},
			maxConcurrent: 0,
			expected: map[string]int{
				"1.1.1.1": 1,
				"2.2.2.2": 1,
				"3.3.3.3": 1,
				"4.4.4.4": 1,
			},
		},
		"should create 1 connection for each instance if max concurrency is > 0 but less than the number of in-use instances": {
			instances: []servicediscovery.Instance{
				{Address: "1.1.1.1", InUse: true},
				{Address: "2.2.2.2", InUse: false},
				{Address: "3.3.3.3", InUse: true},
				{Address: "4.4.4.4", InUse: false},
			},
			maxConcurrent: 1,
			expected: map[string]int{
				"1.1.1.1": 1,
				"2.2.2.2": 1,
				"3.3.3.3": 1,
				"4.4.4.4": 1,
			},
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			cfg := Config{
				MaxConcurrentRequests: testData.maxConcurrent,
			}

			w, err := newQuerierWorkerWithProcessor(cfg, log.NewNopLogger(), &mockProcessor{}, nil, nil)
			require.NoError(t, err)

			for _, instance := range testData.instances {
				w.instances[instance.Address] = instance
			}

			assert.Equal(t, testData.expected, w.getDesiredConcurrency())
		})
	}
}

func getConcurrentProcessors(w *querierWorker) int {
	result := 0
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, mgr := range w.managers {
		result += int(mgr.currentProcessors.Load())
	}

	return result
}

type mockProcessor struct{}

func (m mockProcessor) processQueriesOnSingleStream(ctx context.Context, _ *grpc.ClientConn, _ string) {
	<-ctx.Done()
}

func (m mockProcessor) notifyShutdown(_ context.Context, _ *grpc.ClientConn, _ string) {}
