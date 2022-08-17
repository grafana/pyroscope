FROM golang:1.18.5 as build

WORKDIR /src/fire

COPY go.mod go.sum /src/fire/

COPY . /src/fire

RUN git config --global url.ssh://git@github.com/.insteadOf https://github.com/
RUN mkdir -p -m 0700 ~/.ssh && ssh-keyscan github.com >> ~/.ssh/known_hosts
RUN --mount=type=ssh make clean go/mod go/bin

FROM alpine:3.16.2

RUN apk add --no-cache ca-certificates

COPY --from=build /src/fire/bin/fire /usr/bin/fire
COPY cmd/fire/fire.yaml /etc/fire/config.yaml

RUN addgroup -g 10001 -S fire && \
    adduser -u 10001 -S fire -G fire
RUN mkdir -p /data && \
    chown -R fire:fire /data
VOLUME /data

USER fire
EXPOSE 4100
ENTRYPOINT [ "/usr/bin/fire" ]
CMD ["-config.file=/etc/fire/config.yaml"]
