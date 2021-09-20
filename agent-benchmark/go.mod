module agent-bechmark

go 1.16

replace github.com/pyroscope-io/pyroscope => ../

require (
	github.com/dhoomakethu/stress v0.0.0-20210419083025-aaf0fe4f03ce // indirect
	github.com/jaegertracing/jaeger v1.26.0
	github.com/pyroscope-io/pyroscope v0.0.39
)
