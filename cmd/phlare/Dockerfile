FROM alpine:3.16.2

RUN apk add --no-cache ca-certificates

COPY cmd/phlare/phlare.yaml /etc/phlare/config.yaml
COPY profilecli /usr/bin/profilecli
COPY phlare /usr/bin/phlare

RUN addgroup -g 10001 -S phlare && \
    adduser -u 10001 -S phlare -G phlare
RUN mkdir -p /data && \
    chown -R phlare:phlare /data
VOLUME /data

USER phlare
EXPOSE 4100
ENTRYPOINT [ "/usr/bin/phlare" ]
CMD ["-config.file=/etc/phlare/config.yaml"]
