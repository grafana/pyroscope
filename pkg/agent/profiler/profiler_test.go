package profiler_test

import (
	"context"
	gopprof "runtime/pprof"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/agent/pprof"
	"github.com/pyroscope-io/pyroscope/pkg/agent/profiler"
)

var _ = Describe("WithLabels", func() {
	RegisterFailHandler(Fail)

	It("Propagates goroutine labels implicitly", func() {
		c := make(chan struct{})
		Expect(pprof.GetGoroutineLabels()).To(Equal(profiler.LabelSet{}))

		profiler.WithLabels(profiler.Labels("foo", "bar"), func() {
			expectLabels(profiler.Labels(
				"foo", "bar",
			))

			profiler.WithLabels(profiler.Labels("baz", "qux"), func() {
				go func() {
					expectLabels(profiler.Labels(
						"foo", "bar",
						"baz", "qux",
					))
					close(c)
				}()
				<-c
			})

			expectLabels(profiler.Labels(
				"foo", "bar",
			))
		})

		Expect(pprof.GetGoroutineLabels()).To(Equal(profiler.LabelSet{}))
	})
})

var _ = Describe("WithLabelsContext", func() {
	RegisterFailHandler(Fail)

	It("Propagates goroutine labels explicitly via context", func() {
		c := make(chan struct{})
		ctx := context.Background()
		Expect(pprof.GetGoroutineLabels()).To(Equal(profiler.LabelSet{}))

		profiler.WithLabelsContext(ctx, profiler.Labels("foo", "bar"), func(ctx context.Context) {
			expectLabels(profiler.Labels(
				"foo", "bar",
			))

			profiler.WithLabelsContext(ctx, profiler.Labels("baz", "qux"), func(ctx context.Context) {
				go func() {
					expectLabels(profiler.Labels(
						"foo", "bar",
						"baz", "qux",
					))
					close(c)
				}()
				<-c
			})

			expectLabels(profiler.Labels(
				"foo", "bar",
			))
		})

		Expect(pprof.GetGoroutineLabels()).To(Equal(profiler.LabelSet{}))
	})
})

var _ = Describe("WithLabelsContext is interchangeable with pprof.Do", func() {
	RegisterFailHandler(Fail)

	It("Preserves API compatibility", func() {
		ctx := context.Background()
		Expect(pprof.GetGoroutineLabels()).To(Equal(profiler.LabelSet{}))

		profiler.WithLabelsContext(ctx, profiler.Labels("foo", "bar"), func(ctx context.Context) {
			expectLabels(profiler.Labels(
				"foo", "bar",
			))

			gopprof.Do(ctx, profiler.Labels("baz", "qux"), func(ctx context.Context) {
				expectLabels(profiler.Labels(
					"foo", "bar",
					"baz", "qux",
				))

				gopprof.Do(ctx, gopprof.Labels("zoo", "ooz"), func(ctx context.Context) {
					expectLabels(profiler.Labels(
						"foo", "bar",
						"baz", "qux",
						"zoo", "ooz",
					))
				})

				profiler.WithLabelsContext(ctx, gopprof.Labels("zoo", "ooz"), func(ctx context.Context) {
					expectLabels(profiler.Labels(
						"foo", "bar",
						"baz", "qux",
						"zoo", "ooz",
					))
				})

				expectLabels(profiler.Labels(
					"foo", "bar",
					"baz", "qux",
				))
			})

			expectLabels(profiler.Labels(
				"foo", "bar",
			))
		})

		Expect(pprof.GetGoroutineLabels()).To(Equal(profiler.LabelSet{}))
	})
})

var _ = Describe("WithLabels should not be used with WithLabelsContext and/or pprof.Do", func() {
	RegisterFailHandler(Fail)

	It("Does not propagate goroutine labels", func() {
		ctx := context.Background()
		Expect(pprof.GetGoroutineLabels()).To(Equal(profiler.LabelSet{}))

		profiler.WithLabels(profiler.Labels("foo", "bar"), func() {
			expectLabels(profiler.Labels(
				"foo", "bar",
			))

			// The ctx is empty meaning that WithLabelsContext/pprof.Do
			// remove/replace the current label set.

			gopprof.Do(ctx, profiler.Labels("baz", "qux"), func(ctx context.Context) {
				expectLabels(profiler.Labels(
					// "foo", "bar",
					"baz", "qux",
				))
			})

			profiler.WithLabelsContext(ctx, gopprof.Labels("baz", "qux"), func(ctx context.Context) {
				expectLabels(profiler.Labels(
					// "foo", "bar",
					"baz", "qux",
				))
			})

			expectLabels(profiler.Labels(
			// "foo", "bar",
			))
		})

		Expect(pprof.GetGoroutineLabels()).To(Equal(profiler.LabelSet{}))
	})
})

func expectLabels(labels profiler.LabelSet) {
	Expect(labelsSlice(pprof.GetGoroutineLabels())).
		To(ConsistOf(labelsSlice(labels)))
}

func labelsSlice(labels profiler.LabelSet) []string {
	var s []string
	ctx := gopprof.WithLabels(context.Background(), labels)
	gopprof.ForLabels(ctx, func(k, v string) bool {
		s = append(s, k+"="+v)
		return true
	})
	return s
}
