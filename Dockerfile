# rust deps build

FROM rust:1.48.0-buster as rust-builder

RUN apt-get update && apt-get install -y libunwind-dev

COPY third_party/rbspy /opt/rbspy
COPY third_party/pyspy /opt/pyspy
COPY third_party/rustdeps /opt/rustdeps

WORKDIR /opt/rustdeps

RUN cargo build --release

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
RUN apt-get update && apt-get install -y make git zstd gcc g++ libc-dev libunwind-dev && rm -rf /var/lib/apt/lists/*

WORKDIR /opt/pyroscope

RUN mkdir -p /opt/pyroscope/third_party/rustdeps/target/release
COPY --from=rust-builder /opt/rustdeps/target/release/librustdeps.a /opt/pyroscope/third_party/rustdeps/target/release/librustdeps.a
COPY --from=rust-builder /opt/rbspy/lib/rbspy.h /opt/pyroscope/third_party/rbspy/lib/
COPY --from=rust-builder /opt/pyspy/lib/pyspy.h /opt/pyroscope/third_party/pyspy/lib/

COPY --from=js-builder /opt/pyroscope/webapp/public ./webapp/public
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

WORKDIR /var/lib/pyroscope

# RUN apk add --no-cache ca-certificates bash tzdata openssl musl-utils
RUN apt-get update && apt-get install -y ca-certificates bash tzdata openssl libunwind8 && rm -rf /var/lib/apt/lists/*

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
ENTRYPOINT [ "/usr/bin/pyroscope", "server" ]
