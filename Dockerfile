# rbspy build

FROM rust:1.48.0-buster as rbspy-builder

WORKDIR /opt/rbspy

COPY third_party/rbspy .

# mock meaning an empty file. this way COPY won't fail down the line
RUN make build

# pyspy build

FROM rust:1.48.0-buster as pyspy-builder

WORKDIR /opt/pyspy

COPY third_party/pyspy .

RUN make build

# assets build
# doesn't matter what arch it is on, hence --platform
FROM --platform=$BUILDPLATFORM node:14.15.1-alpine3.12 as js-builder

RUN apk add --no-cache make

WORKDIR /opt/pyroscope

COPY package.json package-lock.json .babelrc Makefile ./
COPY scripts ./scripts
COPY webapp ./webapp

RUN make assets

# go build

FROM golang:1.15.1-buster as go-builder

# RUN apk add --no-cache make git zstd gcc g++ libc-dev musl-dev
RUN apt-get update && apt-get install -y make git zstd gcc g++ libc-dev && rm -rf /var/lib/apt/lists/*

WORKDIR /opt/pyroscope

COPY --from=js-builder /opt/pyroscope/webapp/public ./webapp/public
COPY --from=rbspy-builder /opt/rbspy/target/release/librbspy.a /opt/pyroscope/third_party/rbspy/lib/librbspy.a
COPY --from=rbspy-builder /opt/rbspy/lib/rbspy.h /opt/pyroscope/third_party/rbspy/lib/
COPY --from=pyspy-builder /opt/pyspy/target/release/libpy_spy.a /opt/pyroscope/third_party/pyspy/lib/libpyspy.a
COPY --from=pyspy-builder /opt/pyspy/lib/pyspy.h /opt/pyroscope/third_party/pyspy/lib/
COPY pkg ./pkg
COPY cmd ./cmd
COPY tools ./tools
COPY scripts ./scripts
COPY go.mod go.sum pyroscope.go ./
COPY Makefile ./

# EXTRA_LDFLAGS="-linkmode external -extldflags \"-static\""
RUN EMBEDDED_ASSETS_DEPS="" make build-release

# final image

# FROM alpine:3.12
FROM debian:buster

LABEL maintainer="Pyroscope team <hello@pyroscope.io>"

WORKDIR $PS_PATHS_HOME

# RUN apk add --no-cache ca-certificates bash tzdata openssl musl-utils
RUN apt-get update && apt-get install -y ca-certificates bash tzdata openssl && rm -rf /var/lib/apt/lists/*

RUN addgroup --system pyroscope && adduser --system pyroscope && adduser pyroscope pyroscope

RUN mkdir -p \
        "/var/lib/pyroscope" \
        "/var/log/pyroscope" \
        "/etc/pyroscope" \
        && \
    chown -R "pyroscope:pyroscope" \
        "/var/lib/pyroscope" \
        "/var/log/pyroscope" \
        "/etc/pyroscope" \
        && \
    chmod -R 777 \
        "/var/lib/pyroscope" \
        "/var/log/pyroscope" \
        "/etc/pyroscope"

COPY scripts/packages/server.yml "/etc/pyroscope/server.yml"
COPY --from=go-builder /opt/pyroscope/bin/pyroscope /usr/bin/pyroscope
RUN chmod 777 /usr/bin/pyroscope

USER pyroscope
EXPOSE 8080/tcp
ENTRYPOINT [ "/opt/pyroscope/bin/pyroscope", "server" ]
