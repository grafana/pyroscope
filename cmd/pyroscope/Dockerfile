FROM gcr.io/distroless/static:debug@sha256:c7f818f4678fe4ceb0412a370e8e41ca373d5dde25039b84f9cccb72ef558542 AS debug

SHELL [ "/busybox/sh", "-c" ]

RUN addgroup -g 10001 -S pyroscope && \
    adduser -u 10001 -S pyroscope -G pyroscope -h /data

FROM gcr.io/distroless/static@sha256:87bce11be0af225e4ca761c40babb06d6d559f5767fbf7dc3c47f0f1a466b92c

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
