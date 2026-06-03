ARG PROFILER_VERSION=1.0.0

# Fetch the profiler artifacts from the GitHub release.
FROM alpine:3 AS sdk
ARG PROFILER_VERSION
ARG TARGETARCH
RUN apk add --no-cache curl tar
RUN case "$TARGETARCH" in \
      amd64) PROFILER_ARCH=x86_64 ;; \
      arm64) PROFILER_ARCH=aarch64 ;; \
      *) echo "unsupported TARGETARCH: $TARGETARCH" >&2; exit 1 ;; \
    esac \
    && curl -fsSL -o /tmp/pyroscope.tar.gz \
      "https://github.com/grafana/pyroscope-dotnet/releases/download/pyroscope-${PROFILER_VERSION}/pyroscope.${PROFILER_VERSION}-musl-${PROFILER_ARCH}.tar.gz" \
    && mkdir -p /pyroscope \
    && tar -xzf /tmp/pyroscope.tar.gz -C /pyroscope

FROM mcr.microsoft.com/dotnet/sdk:6.0-alpine

WORKDIR /dotnet

COPY --from=sdk /pyroscope/Pyroscope.Profiler.Native.so ./Pyroscope.Profiler.Native.so
COPY --from=sdk /pyroscope/Pyroscope.Linux.ApiWrapper.x64.so ./Pyroscope.Linux.ApiWrapper.x64.so

ADD example .

RUN dotnet publish -o . -r $(dotnet --info | grep RID | cut -b 6- | tr -d ' ')

ENV CORECLR_ENABLE_PROFILING=1
ENV CORECLR_PROFILER={BD1A650D-AC5D-4896-B64F-D6FA25D6B26A}
ENV CORECLR_PROFILER_PATH=/dotnet/Pyroscope.Profiler.Native.so
ENV LD_PRELOAD=/dotnet/Pyroscope.Linux.ApiWrapper.x64.so


ENV PYROSCOPE_APPLICATION_NAME=fast-slow.dotnet.app
ENV PYROSCOPE_SERVER_ADDRESS=http://pyroscope:4040/
ENV PYROSCOPE_LOG_LEVEL=debug
ENV PYROSCOPE_PROFILING_ENABLED=1
ENV PYROSCOPE_PROFILING_ALLOCATION_ENABLED=true
ENV PYROSCOPE_PROFILING_CONTENTION_ENABLED=true
ENV PYROSCOPE_PROFILING_EXCEPTION_ENABLED=true


CMD ["/dotnet/example"]
