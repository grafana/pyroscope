FROM gcr.io/distroless/static:debug

SHELL [ "/busybox/sh", "-c" ]

RUN addgroup -g 10001 -S pyroscope && \
    adduser -u 10001 -S pyroscope -G pyroscope -h /data

# This folder is created by adduser command with right owner/group
VOLUME /data

# This folder needs to be created and set to the right owner/group
VOLUME /data-compactor
RUN mkdir -p /data-compactor && chown pyroscope:pyroscope /data /data-compactor

COPY .tmp/bin/dlv /usr/bin/dlv
COPY cmd/pyroscope/pyroscope.yaml /etc/pyroscope/config.yaml
COPY profilecli /usr/bin/profilecli
COPY pyroscope /usr/bin/pyroscope

USER pyroscope
EXPOSE 4040
ENTRYPOINT ["/usr/bin/dlv", "--listen=:40000", "--headless=true", "--log", "--continue", "--accept-multiclient" , "--api-version=2", "exec", "/usr/bin/pyroscope", "--"]
CMD ["-config.file=/etc/pyroscope/config.yaml"]
