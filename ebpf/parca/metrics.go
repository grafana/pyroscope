// Copyright 2022-2024 The Parca Authors
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
//

package parca

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	labelUser        = "user"
	labelKernel      = "kernel"
	labelInterpreter = "interpreter"

	labelKernelUnwind      = "kernel_unwind"
	labelInterpreterUnwind = "interpreter_unwind"
	labelNativeUnwind      = "native_unwind"

	labelError   = "error"
	labelMissing = "missing"
	labelFailed  = "failed"
	labelSuccess = "success"

	labelStackDropReasonKey       = "read_stack_key"
	labelStackDropReasonUser      = "read_user_stack"
	labelStackDropReasonKernel    = "read_kernel_stack"
	labelStackDropReasonCount     = "read_stack_count"
	labelStackDropReasonZeroCount = "read_stack_count_zero"
	labelStackDropReasonIterator  = "iterator"

	labelEventEmpty           = "empty"
	labelEventUnwindInfo      = "unwind_info"
	labelEventProcessMappings = "process_mappings"
	labelEventRefreshProcInfo = "refresh_proc_info"

	labelProfileDropReasonProcessInfo = "process_info"

	labelNeedMoreProfilingRounds = "need_more_rounds"
	labelProcfsRace              = "procfs_race"
	labelTooManyMappings         = "too_many_mappings"
	labelOther                   = "other"
)

type metrics struct {
	// profile level.
	obtainAttempts *prometheus.CounterVec
	obtainDuration prometheus.Histogram
	profileDrop    *prometheus.CounterVec

	// stack level.
	stackDrop       *prometheus.CounterVec
	readMapAttempts *prometheus.CounterVec

	// event level.
	eventsReceived *prometheus.CounterVec
	eventsLost     prometheus.Counter

	unwindTableAddErrors     *prometheus.CounterVec
	unwindTablePersistErrors *prometheus.CounterVec
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		obtainAttempts: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name:        "parca_agent_profiler_attempts_total",
				Help:        "Total number of attempts to obtain a profile.",
				ConstLabels: map[string]string{"type": "cpu"},
			},
			[]string{"status"},
		),
		obtainDuration: promauto.With(reg).NewHistogram(
			prometheus.HistogramOpts{
				Name:                        "parca_agent_profiler_attempt_duration_seconds",
				Help:                        "The duration it takes to collect profiles from the BPF maps",
				ConstLabels:                 map[string]string{"type": "cpu"},
				NativeHistogramBucketFactor: 1.1,
			},
		),
		stackDrop: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name:        "parca_agent_profiler_stack_drop_total",
				Help:        "Total number of stacks dropped from the profile.",
				ConstLabels: map[string]string{"type": "cpu"},
			},
			[]string{"reason"},
		),
		readMapAttempts: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name:        "parca_agent_profiler_map_read_attempts_total",
				Help:        "Number of attempts to read from the BPF maps.",
				ConstLabels: map[string]string{"type": "cpu"},
			},
			[]string{"stack", "action", "status"},
		),
		profileDrop: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name:        "parca_agent_profiler_profiles_drop_total",
				Help:        "Number of profiles dropped from the profile (one profile represents 1 process in a profiling duration).",
				ConstLabels: map[string]string{"type": "cpu"},
			},
			[]string{"reason"},
		),
		eventsReceived: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name:        "parca_agent_profiler_events_received_total",
				Help:        "Total number of profile events received.",
				ConstLabels: map[string]string{"type": "cpu"},
			},
			[]string{"event"}),
		eventsLost: promauto.With(reg).NewCounter(
			prometheus.CounterOpts{
				Name:        "parca_agent_profiler_events_lost_total",
				Help:        "Total number of profile events lost.",
				ConstLabels: map[string]string{"type": "cpu"},
			},
		),
		unwindTableAddErrors: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name:        "parca_agent_profiler_unwind_table_add_errors_total",
				Help:        "Total number of errors adding entries to the unwind table.",
				ConstLabels: map[string]string{"type": "cpu"},
			},
			[]string{"error"}),
		unwindTablePersistErrors: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name:        "parca_agent_profiler_unwind_table_persist_errors_total",
				Help:        "Total number of errors persisting the unwind table.",
				ConstLabels: map[string]string{"type": "cpu"},
			},
			[]string{"error"}),
	}
	m.obtainAttempts.WithLabelValues(labelSuccess)
	m.obtainAttempts.WithLabelValues(labelError)

	m.stackDrop.WithLabelValues(labelStackDropReasonKey)
	m.stackDrop.WithLabelValues(labelStackDropReasonUser)
	m.stackDrop.WithLabelValues(labelStackDropReasonKernel)
	m.stackDrop.WithLabelValues(labelStackDropReasonCount)
	m.stackDrop.WithLabelValues(labelStackDropReasonZeroCount)
	m.stackDrop.WithLabelValues(labelStackDropReasonIterator)

	m.readMapAttempts.WithLabelValues(labelUser, labelNativeUnwind, labelSuccess)
	m.readMapAttempts.WithLabelValues(labelUser, labelNativeUnwind, labelError)
	m.readMapAttempts.WithLabelValues(labelUser, labelNativeUnwind, labelMissing)
	m.readMapAttempts.WithLabelValues(labelUser, labelNativeUnwind, labelFailed)

	m.readMapAttempts.WithLabelValues(labelUser, labelKernelUnwind, labelSuccess)
	m.readMapAttempts.WithLabelValues(labelUser, labelKernelUnwind, labelError)
	m.readMapAttempts.WithLabelValues(labelUser, labelKernelUnwind, labelMissing)
	m.readMapAttempts.WithLabelValues(labelUser, labelKernelUnwind, labelFailed)

	m.readMapAttempts.WithLabelValues(labelKernel, labelKernelUnwind, labelSuccess)
	m.readMapAttempts.WithLabelValues(labelKernel, labelKernelUnwind, labelError)
	m.readMapAttempts.WithLabelValues(labelKernel, labelKernelUnwind, labelMissing)
	m.readMapAttempts.WithLabelValues(labelKernel, labelKernelUnwind, labelFailed)

	m.profileDrop.WithLabelValues(labelProfileDropReasonProcessInfo)

	m.eventsReceived.WithLabelValues(labelEventEmpty)
	m.eventsReceived.WithLabelValues(labelEventUnwindInfo)
	m.eventsReceived.WithLabelValues(labelEventProcessMappings)
	m.eventsReceived.WithLabelValues(labelEventRefreshProcInfo)

	m.unwindTableAddErrors.WithLabelValues(labelNeedMoreProfilingRounds)
	m.unwindTableAddErrors.WithLabelValues(labelProcfsRace)
	m.unwindTableAddErrors.WithLabelValues(labelTooManyMappings)
	m.unwindTableAddErrors.WithLabelValues(labelOther)

	m.unwindTablePersistErrors.WithLabelValues(labelNeedMoreProfilingRounds)
	m.unwindTablePersistErrors.WithLabelValues(labelOther)

	return m
}

const (
	labelHash           = "hash"
	labelUnwindTableAdd = "unwind_table_add"
)

type MapsMetrics struct {
	refreshProcessInfoErrors *prometheus.CounterVec

	// Map clean.
	mapCleanErrors *prometheus.CounterVec
}

func NewMapsMetrics(reg prometheus.Registerer) *MapsMetrics {
	m := &MapsMetrics{
		refreshProcessInfoErrors: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name:        "parca_agent_profiler_bpf_maps_refresh_proc_info_errors_total",
			Help:        "Number of errors refreshing process info",
			ConstLabels: map[string]string{"type": "cpu"},
		}, []string{"error"}),
		mapCleanErrors: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name:        "parca_agent_profiler_bpf_maps_clean_errors_total",
			Help:        "Number of errors cleaning BPF maps",
			ConstLabels: map[string]string{"type": "cpu"},
		}, []string{"map"}),
	}

	m.refreshProcessInfoErrors.WithLabelValues(labelHash)
	m.refreshProcessInfoErrors.WithLabelValues(labelUnwindTableAdd)

	m.mapCleanErrors.WithLabelValues(StackTracesMapName)
	m.mapCleanErrors.WithLabelValues(StackCountsMapName)
	m.mapCleanErrors.WithLabelValues(ProcessInfoMapName)
	m.mapCleanErrors.WithLabelValues(UnwindInfoChunksMapName)
	return m
}
