FROM golang as builder

WORKDIR /app
FROM alpine:3.18.6

RUN apk add --no-cache ca-certificates

COPY .tmp/bin/linux_amd64/dlv /usr/bin/dlv

COPY cmd/pyroscope/pyroscope.yaml /etc/pyroscope/config.yaml
COPY profilecli /usr/bin/profilecli
COPY pyroscope /usr/bin/pyroscope

RUN addgroup -g 10001 -S pyroscope && \
    adduser -u 10001 -S pyroscope -G pyroscope
RUN mkdir -p /data && \
    chown -R pyroscope:pyroscope /data
VOLUME /data

USER pyroscope
EXPOSE 4040
ENTRYPOINT ["/usr/bin/dlv", "--listen=:40000", "--headless=true", "--log", "--continue", "--accept-multiclient" , "--api-version=2", "exec", "/usr/bin/pyroscope", "--"]
CMD ["-config.file=/etc/pyroscope/config.yaml"]
