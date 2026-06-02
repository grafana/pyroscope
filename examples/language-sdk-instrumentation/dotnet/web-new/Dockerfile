ARG PROFILER_VERSION=0.15.0

# Fetch the profiler artifacts from the GitHub release.
FROM --platform=linux/amd64 alpine:3 AS sdk
ARG PROFILER_VERSION
RUN apk add --no-cache curl tar
RUN curl -fsSL -o /tmp/pyroscope.tar.gz \
      "https://github.com/grafana/pyroscope-dotnet/releases/download/v${PROFILER_VERSION}-pyroscope/pyroscope.${PROFILER_VERSION}-glibc-x86_64.tar.gz" \
    && mkdir -p /pyroscope \
    && tar -xzf /tmp/pyroscope.tar.gz -C /pyroscope

FROM --platform=linux/amd64 mcr.microsoft.com/dotnet/sdk:6.0
# FROM --platform=linux/amd64 mcr.microsoft.com/dotnet/sdk:6.0-alpine

WORKDIR /dotnet

COPY --from=sdk /pyroscope/Pyroscope.Profiler.Native.so ./Pyroscope.Profiler.Native.so
COPY --from=sdk /pyroscope/Pyroscope.Linux.ApiWrapper.x64.so ./Pyroscope.Linux.ApiWrapper.x64.so

ADD example .

RUN dotnet publish -o . -r $(dotnet --info | grep RID | cut -b 6- | tr -d ' ')

ENV CORECLR_ENABLE_PROFILING=1
ENV CORECLR_PROFILER={BD1A650D-AC5D-4896-B64F-D6FA25D6B26A}
ENV CORECLR_PROFILER_PATH=/dotnet/Pyroscope.Profiler.Native.so
ENV LD_PRELOAD=/dotnet/Pyroscope.Linux.ApiWrapper.x64.so
ENV LD_LIBRARY_PATH=/dotnet

ENV PYROSCOPE_APPLICATION_NAME=web.dotnet.app
ENV PYROSCOPE_SERVER_ADDRESS=http://pyroscope:4040
ENV PYROSCOPE_LOG_LEVEL=debug
ENV PYROSCOPE_PROFILING_ENABLED=1
ENV PYROSCOPE_PROFILING_ALLOCATION_ENABLED=true
ENV PYROSCOPE_PROFILING_CONTENTION_ENABLED=true
ENV PYROSCOPE_PROFILING_EXCEPTION_ENABLED=true

CMD ["dotnet", "/dotnet/example.dll"]
