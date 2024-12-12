FROM ubuntu:22.04

RUN apt-get update && \
    apt-get install -y linux-headers-generic && \
    rm -rf /var/lib/apt/lists/*

# clone sha
COPY ebpf-profiler /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/ebpf-profiler", "-collection-agent", "otel-collector-service:4317", "-no-kernel-version-check", "-disable-tls"]
