FROM gcr.io/distroless/static:debug@sha256:fdd11b2ee83d2cd35774fcb1c94c76e57fd4470da0c26d5748a91fc2faf7b9b1 AS debug

SHELL [ "/busybox/sh", "-c" ]

RUN addgroup -g 10001 -S pyroscope && \
    adduser -u 10001 -S pyroscope -G pyroscope -h /data

FROM gcr.io/distroless/static@sha256:9e0130bec991f471279b70f1630ada11e46dad439ef2949544f96001225329fb

COPY --from=debug /etc/passwd /etc/passwd
COPY --from=debug /etc/group /etc/group

# Copy folder from debug container, this folder needs to have the correct UID
# in order for the container to run as non-root.
VOLUME /data
COPY --chown=pyroscope:pyroscope --from=debug /data /data
VOLUME /data-compactor
COPY --chown=pyroscope:pyroscope --from=debug /data /data-compactor
VOLUME /data-metastore
COPY --chown=pyroscope:pyroscope --from=debug /data /data-metastore

COPY cmd/pyroscope/pyroscope.yaml /etc/pyroscope/config.yaml
COPY profilecli /usr/bin/profilecli
COPY pyroscope /usr/bin/pyroscope

USER pyroscope
EXPOSE 4040
ENTRYPOINT [ "/usr/bin/pyroscope" ]
CMD ["-config.file=/etc/pyroscope/config.yaml"]
