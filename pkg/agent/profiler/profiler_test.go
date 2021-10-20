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
		Expect(pprof.GetGoroutineLabels()).To(BeEmpty())

		profiler.WithLabels(profiler.Labels{"foo": "bar"}, func() {
			expectLabels(profiler.Labels{
				"foo": "bar",
			})

			profiler.WithLabels(profiler.Labels{"baz": "qux"}, func() {
				c := make(chan struct{})
				go func() {
					expectLabels(profiler.Labels{
						"foo": "bar",
						"baz": "qux",
					})
					close(c)
				}()
				<-c
			})

			expectLabels(profiler.Labels{
				"foo": "bar",
			})
		})

		Expect(pprof.GetGoroutineLabels()).To(BeEmpty())
	})
})

var _ = Describe("SetLabels", func() {
	RegisterFailHandler(Fail)

	It("Overrides goroutine labels explicitly", func() {
		Expect(pprof.GetGoroutineLabels()).To(BeEmpty())

		profiler.WithLabels(profiler.Labels{"foo": "bar"}, func() {
			expectLabels(profiler.Labels{
				"foo": "bar",
			})

			profiler.WithLabels(profiler.Labels{"baz": "qux"}, func() {
				expectLabels(profiler.Labels{
					"foo": "bar",
					"baz": "qux",
				})
				profiler.SetLabels(profiler.Labels{
					"zoo": "ooz",
				})
				expectLabels(profiler.Labels{
					"zoo": "ooz",
				})
			})

			expectLabels(profiler.Labels{
				"foo": "bar",
			})
		})

		Expect(pprof.GetGoroutineLabels()).To(BeEmpty())
	})
})

var _ = Describe("Interoperability with runtime/pprof", func() {
	RegisterFailHandler(Fail)

	It("Does not propagate goroutine labels implicitly", func() {
		ctx := context.Background()
		Expect(pprof.GetGoroutineLabels()).To(BeEmpty())

		profiler.WithLabels(profiler.Labels{"foo": "bar"}, func() {
			expectLabels(profiler.Labels{
				"foo": "bar",
			})

			// The ctx is empty meaning that Do/pprof.Do
			// remove/replace the current label set.
			gopprof.Do(ctx, gopprof.Labels("baz", "qux"), func(ctx context.Context) {
				expectLabels(profiler.Labels{
					// "foo": "bar",
					"baz": "qux",
				})
			})

			Expect(pprof.GetGoroutineLabels()).To(BeEmpty())
			// It would contain foo=bar label.
		})

		Expect(pprof.GetGoroutineLabels()).To(BeEmpty())
	})

	It("Propagates goroutine labels explicitly via Context", func() {
		ctx := context.Background()
		Expect(pprof.GetGoroutineLabels()).To(BeEmpty())

		profiler.WithLabels(profiler.Labels{"foo": "bar"}, func() {
			expectLabels(profiler.Labels{
				"foo": "bar",
			})

			// The ctx is enriched with the current labels.
			gopprof.Do(profiler.Context(ctx), gopprof.Labels("baz", "qux"), func(ctx context.Context) {
				expectLabels(profiler.Labels{
					"foo": "bar",
					"baz": "qux",
				})

				profiler.WithLabels(profiler.Labels{"zoo": "ooz"}, func() {
					c := make(chan struct{})
					go func() {
						expectLabels(profiler.Labels{
							"foo": "bar",
							"baz": "qux",
							"zoo": "ooz",
						})
						close(c)
					}()
					<-c
				})

				expectLabels(profiler.Labels{
					"foo": "bar",
					"baz": "qux",
				})
			})

			expectLabels(profiler.Labels{
				"foo": "bar",
			})
		})

		Expect(pprof.GetGoroutineLabels()).To(BeEmpty())
	})
})

func expectLabels(labels map[string]string) {
	Expect(pprof.GetGoroutineLabels()).To(Equal(labels))
}
