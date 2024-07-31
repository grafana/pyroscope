FROM gcr.io/distroless/static:debug AS debug

SHELL [ "/busybox/sh", "-c" ]

RUN addgroup -g 10001 -S pyroscope && \
    adduser -u 10001 -S pyroscope -G pyroscope -h /data

FROM gcr.io/distroless/static

COPY --from=debug /etc/passwd /etc/passwd
COPY --from=debug /etc/group /etc/group

# Copy folder from debug container, this folder needs to have the correct UID
# in order for the container to run as non-root.
VOLUME /data
COPY --chown=pyroscope:pyroscope --from=debug /data /data
VOLUME /data-compactor
COPY --chown=pyroscope:pyroscope --from=debug /data /data-compactor

COPY cmd/pyroscope/pyroscope.yaml /etc/pyroscope/config.yaml
COPY profilecli /usr/bin/profilecli
COPY pyroscope /usr/bin/pyroscope

USER pyroscope
EXPOSE 4040
ENTRYPOINT [ "/usr/bin/pyroscope" ]
CMD ["-config.file=/etc/pyroscope/config.yaml"]
