FROM gcr.io/distroless/static:debug@sha256:63b4c0065ff7bd68753510400b2d5cedd5d5571d15eac4be0b6b0dd236a2d148 AS debug

SHELL [ "/busybox/sh", "-c" ]

RUN addgroup -g 10001 -S pyroscope && \
    adduser -u 10001 -S pyroscope -G pyroscope -h /data

FROM gcr.io/distroless/static@sha256:448810357cc8e67b7208988abdba0f5ec3d3aebee0a5b13eb624979792cffc0f

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
