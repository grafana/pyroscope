FROM aspdcore.azurecr.io/dotnet-8/sdk:8.0.638

WORKDIR /dotnet

COPY --from=pyroscope/pyroscope-dotnet:0.10.0-musl /Pyroscope.Profiler.Native.so ./Pyroscope.Profiler.Native.so
COPY --from=pyroscope/pyroscope-dotnet:0.10.0-musl /Pyroscope.Linux.ApiWrapper.x64.so ./Pyroscope.Linux.ApiWrapper.x64.so

ADD example .

RUN dotnet publish --configuration Release --output publish --no-self-contained --runtime linux-musl-x64 -p:Version=2.3.9

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


CMD ["dotnet", "publish/example.dll"]
