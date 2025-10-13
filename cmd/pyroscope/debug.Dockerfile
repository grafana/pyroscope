FROM gcr.io/distroless/static:debug@sha256:c7f818f4678fe4ceb0412a370e8e41ca373d5dde25039b84f9cccb72ef558542

SHELL [ "/busybox/sh", "-c" ]

RUN addgroup -g 10001 -S pyroscope && \
    adduser -u 10001 -S pyroscope -G pyroscope -h /data

# This folder is created by adduser command with right owner/group
VOLUME /data
VOLUME /data-compactor
VOLUME /data-metastore
RUN chown pyroscope:pyroscope /data /data-compactor /data-metastore

COPY .tmp/bin/dlv /usr/bin/dlv
COPY cmd/pyroscope/pyroscope.yaml /etc/pyroscope/config.yaml
COPY profilecli /usr/bin/profilecli
COPY pyroscope /usr/bin/pyroscope

USER pyroscope
EXPOSE 4040
ENTRYPOINT ["/usr/bin/dlv", "--listen=:40000", "--headless=true", "--log", "--continue", "--accept-multiclient" , "--api-version=2", "exec", "/usr/bin/pyroscope", "--"]
CMD ["-config.file=/etc/pyroscope/config.yaml"]
