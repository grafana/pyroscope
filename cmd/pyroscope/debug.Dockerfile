FROM gcr.io/distroless/static:debug@sha256:7dc183cc0aea6abd9d105135e49d37b7474a79391ebea7eb55557cd4486d2225

SHELL [ "/busybox/sh", "-c" ]

RUN addgroup -g 10001 -S pyroscope && \
    adduser -u 10001 -S pyroscope -G pyroscope -h /data

# Ensure folders are created correctly
VOLUME /data
VOLUME /data-compactor
VOLUME /data-metastore
RUN mkdir -p /data /data-compactor /data-metastore && \
    chown pyroscope:pyroscope /data /data-compactor /data-metastore

COPY .tmp/bin/dlv /usr/bin/dlv
COPY cmd/pyroscope/pyroscope.yaml /etc/pyroscope/config.yaml
COPY profilecli /usr/bin/profilecli
COPY pyroscope /usr/bin/pyroscope

USER pyroscope
EXPOSE 4040
ENTRYPOINT ["/usr/bin/dlv", "--listen=:40000", "--headless=true", "--log", "--continue", "--accept-multiclient" , "--api-version=2", "exec", "/usr/bin/pyroscope", "--"]
CMD ["-config.file=/etc/pyroscope/config.yaml"]
