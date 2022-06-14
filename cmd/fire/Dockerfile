FROM golang:1.18.2 as build

WORKDIR /src/fire

COPY go.mod go.sum /src/fire/
RUN go mod download

COPY . /src/fire

RUN make clean build

FROM alpine:3.16.0

RUN apk add --no-cache ca-certificates

COPY --from=build /src/fire/bin/fire /usr/bin/fire
COPY cmd/fire/fire.yaml /etc/fire/config.yaml

RUN addgroup -g 10001 -S fire && \
    adduser -u 10001 -S fire -G fire
RUN mkdir -p /fire/rules && \
    mkdir -p /fire/rules-temp && \
    chown -R fire:fire /etc/fire /fire

USER fire
EXPOSE 4100
ENTRYPOINT [ "/usr/bin/fire" ]
CMD ["-config.file=/etc/fire/config.yaml"]
