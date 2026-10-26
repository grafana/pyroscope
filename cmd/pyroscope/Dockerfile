FROM gcr.io/distroless/static:debug@sha256:11c839f18a0d7b91b093cda53a5d4cee84f23ec450a6f6e1bf17ab12bf296333 AS debug

SHELL [ "/busybox/sh", "-c" ]

RUN addgroup -g 10001 -S pyroscope && \
    adduser -u 10001 -S pyroscope -G pyroscope -h /data

FROM gcr.io/distroless/static@sha256:034df5c1737e8241f6ed6423a12e720ee0f6d6f088609ee172f98f6f2166ef5d

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
