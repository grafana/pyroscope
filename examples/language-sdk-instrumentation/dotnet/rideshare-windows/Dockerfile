# escape=`
# Windows-container build of the rideshare app with the Pyroscope profiler.
# Build this on a Docker engine in Windows-containers mode.
#
# WINDOWS_BASE_TAG picks the Windows base image variant. The ltsc2022 default
# runs everywhere under Docker Desktop (Hyper-V isolation); on a Windows
# Server host with process isolation it must match the host version, e.g.
# nanoserver-ltsc2025 on Server 2025. The plain multi-arch tags (e.g. `8.0`)
# do not resolve on ltsc2025 hosts, hence the explicit tag.
ARG SDK_VERSION=8.0
ARG WINDOWS_BASE_TAG=nanoserver-ltsc2022
FROM mcr.microsoft.com/dotnet/sdk:$SDK_VERSION-$WINDOWS_BASE_TAG AS build
ARG SDK_VERSION
# nanoserver runs as the non-admin ContainerUser, which cannot write outside
# its profile; this stage is build-only, so run it as admin.
USER ContainerAdministrator

WORKDIR /dotnet

COPY example/BikeService.cs `
     example/CarService.cs `
     example/Example.csproj `
     example/OrderService.cs `
     example/Program.cs `
     example/ScooterService.cs ./

# The -p:TargetFramework override retargets the project to SDK_VERSION without
# editing the csproj (global MSBuild properties beat the file's value).
RUN pwsh -Command "dotnet publish -o out --runtime win-x64 --no-self-contained -p:TargetFramework=net$env:SDK_VERSION"

# Fetch the Windows profiler (a single native DLL) from the GitHub release.
FROM mcr.microsoft.com/dotnet/sdk:$SDK_VERSION-$WINDOWS_BASE_TAG AS profiler
ARG PROFILER_VERSION=1.3.0
USER ContainerAdministrator
RUN pwsh -Command " `
    Invoke-WebRequest -Uri ('https://github.com/grafana/pyroscope-dotnet/releases/download/pyroscope-' + $env:PROFILER_VERSION + '/pyroscope.' + $env:PROFILER_VERSION + '-windows-x64.zip') -OutFile C:/pyroscope.zip; `
    Expand-Archive C:/pyroscope.zip -DestinationPath C:/pyroscope"

FROM mcr.microsoft.com/dotnet/aspnet:$SDK_VERSION-$WINDOWS_BASE_TAG

WORKDIR /dotnet

COPY --from=profiler C:/pyroscope/Pyroscope.Profiler.Native.dll ./Pyroscope.Profiler.Native.dll
COPY --from=build C:/dotnet/out ./

ENV CORECLR_ENABLE_PROFILING=1
ENV CORECLR_PROFILER={BD1A650D-AC5D-4896-B64F-D6FA25D6B26A}
ENV CORECLR_PROFILER_PATH=C:/dotnet/Pyroscope.Profiler.Native.dll

ENV PYROSCOPE_APPLICATION_NAME=rideshare.dotnet.windows.app
ENV PYROSCOPE_SERVER_ADDRESS=http://host.docker.internal:4040
ENV PYROSCOPE_LOG_LEVEL=debug
ENV PYROSCOPE_PROFILING_ENABLED=1
ENV PYROSCOPE_PROFILING_ALLOCATION_ENABLED=true
ENV PYROSCOPE_PROFILING_CONTENTION_ENABLED=true
ENV PYROSCOPE_PROFILING_EXCEPTION_ENABLED=true
ENV PYROSCOPE_PROFILING_HEAP_ENABLED=true
ENV ASPNETCORE_URLS=http://*:5000

ENTRYPOINT ["dotnet", "example.dll"]
