FROM ubuntu:22.04 as builder
RUN apt-get update && apt-get -y install wget gcc
RUN wget https://go.dev/dl/go1.22.10.linux-amd64.tar.gz
RUN tar -C /usr/local -xzf go1.22.10.linux-amd64.tar.gz
# fixed versions for pyroscope, otel-collector, otel-profiler due to protocol changes
RUN wget https://github.com/open-telemetry/opentelemetry-ebpf-profiler/archive/9eb454541a3cf1282a8755dbcec9a9471b243e83.tar.gz
RUN mkdir /profiler
RUN tar --strip-components=1 -C /profiler -xzf 9eb454541a3cf1282a8755dbcec9a9471b243e83.tar.gz
WORKDIR /profiler
RUN /usr/local/go/bin/go build .

FROM ubuntu:22.04

RUN apt-get update && \
    apt-get install -y linux-headers-generic && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /profiler/ebpf-profiler /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/ebpf-profiler"]

