package cputimer

import (
	"context"
	"runtime/pprof"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_CPUTimer_overlap(t *testing.T) {
	ac := NewCPUTimerVec(Opts{
		Name: "test_func_a_cpu_time_total",
	}, []string{"label"})

	bc := NewCPUTimerVec(Opts{
		Name: "test_func_b_cpu_time_total",
	}, []string{"label"})

	ctx := context.Background()

	timer := ac.WithLabelValues("test-a")
	ctx, stop := timer.Start(ctx)
	defer stop()

	assertPprofLabels(t, ctx, map[string]string{
		`__m_{__name__="test_func_a_cpu_time_total", label="test-a"}`: "",
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		bc.WithLabelValues("test-b").Do(ctx, func(ctx context.Context) {
			assertPprofLabels(t, ctx, map[string]string{
				`__m_{__name__="test_func_a_cpu_time_total", label="test-a"}`: "",
				`__m_{__name__="test_func_b_cpu_time_total", label="test-b"}`: "",
			})
		})
		ac.WithLabelValues("test-b").Do(ctx, func(ctx context.Context) {
			assertPprofLabels(t, ctx, map[string]string{
				`__m_{__name__="test_func_a_cpu_time_total", label="test-a"}`: "",
				`__m_{__name__="test_func_a_cpu_time_total", label="test-b"}`: "",
			})
		})
	}()

	assertPprofLabels(t, ctx, map[string]string{
		`__m_{__name__="test_func_a_cpu_time_total", label="test-a"}`: "",
	})

	<-done
}

func assertPprofLabels(t *testing.T, ctx context.Context, expected map[string]string) {
	t.Helper()
	actual := make(map[string]string)
	gatherPprofLabels(ctx, actual)
	assert.Equal(t, expected, actual)
}

func gatherPprofLabels(ctx context.Context, dst map[string]string) {
	pprof.ForLabels(ctx, func(key, value string) bool {
		dst[key] = value
		return true
	})
}
