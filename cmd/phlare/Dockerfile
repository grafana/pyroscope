FROM golang:1.19.2 as build

WORKDIR /src/phlare

COPY go.mod go.sum /src/phlare/

COPY . /src/phlare

RUN git config --global url.ssh://git@github.com/.insteadOf https://github.com/
RUN mkdir -p -m 0700 ~/.ssh && ssh-keyscan github.com >> ~/.ssh/known_hosts
RUN --mount=type=ssh make clean go/mod go/bin

FROM alpine:3.16.2

RUN apk add --no-cache ca-certificates

COPY --from=build /src/phlare/bin/phlare /usr/bin/phlare
COPY --from=build /src/phlare/bin/profilecli /usr/bin/profilecli
COPY cmd/phlare/phlare.yaml /etc/phlare/config.yaml

RUN addgroup -g 10001 -S phlare && \
    adduser -u 10001 -S phlare -G phlare
RUN mkdir -p /data && \
    chown -R phlare:phlare /data
VOLUME /data

USER phlare
EXPOSE 4100
ENTRYPOINT [ "/usr/bin/phlare" ]
CMD ["-config.file=/etc/phlare/config.yaml"]
