FROM --platform=linux/amd64 mcr.microsoft.com/dotnet/sdk:6.0

WORKDIR /dotnet

RUN wget https://github.com/pyroscope-io/pyroscope-dotnet/releases/download/v0.7.2-pyroscope/Pyroscope.Linux.ApiWrapper.x64.so
RUN wget https://github.com/pyroscope-io/pyroscope-dotnet/releases/download/v0.7.2-pyroscope/Pyroscope.Profiler.Native.so

ADD example .

RUN dotnet publish -o . -r $(dotnet --info | grep RID | cut -b 6- | tr -d ' ')

ENV PYROSCOPE_APPLICATION_NAME=tolyan.rideshare.dotnet.app.fargate
ENV PYROSCOPE_SERVER_ADDRESS=http://pyroscope:4040
ENV PYROSCOPE_LOG_LEVEL=debug
ENV CORECLR_ENABLE_PROFILING=1
ENV CORECLR_PROFILER={BD1A650D-AC5D-4896-B64F-D6FA25D6B26A}
ENV CORECLR_PROFILER_PATH=/dotnet/Pyroscope.Profiler.Native.so
ENV LD_PRELOAD=/dotnet/Pyroscope.Linux.ApiWrapper.x64.so
ENV PROFILING_ENABLED=1
ENV PROFILING_ALLOCATION_ENABLED=true
ENV PROFILING_CONTENTION_ENABLED=true
ENV PROFILING_EXCEPTION_ENABLED=true


RUN adduser --disabled-password --gecos --quiet pyroscope
RUN mkdir /var/log/pyroscope && chown pyroscope /var/log/pyroscope
USER pyroscope

CMD ["dotnet", "/dotnet/example.dll"]
