FROM mcr.microsoft.com/dotnet/sdk:3.1

WORKDIR /dotnet

COPY --from=pyroscope/pyroscope:dev /usr/bin/pyroscope /usr/bin/pyroscope
ADD example .

RUN dotnet publish -o . -r $(dotnet --info | grep RID | cut -b 6- | tr -d ' ')

ENV PYROSCOPE_APPLICATION_NAME=web.dotnet.app
ENV PYROSCOPE_SERVER_ADDRESS=http://pyroscope:4040/
ENV PYROSCOPE_LOG_LEVEL=debug
CMD ["pyroscope", "exec", "dotnet", "/dotnet/example.dll"]
