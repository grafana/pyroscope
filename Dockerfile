# assets build

FROM --platform=$BUILDPLATFORM node:14.15.1-alpine3.12 as js-builder

RUN apk add --no-cache make

WORKDIR /opt/pyroscope

COPY package.json package-lock.json .babelrc Makefile ./
COPY scripts ./scripts
COPY webapp ./webapp

RUN make assets

# go build

FROM --platform=$BUILDPLATFORM golang:1.15.1-alpine3.12 as go-builder

RUN apk add --no-cache make git zstd gcc g++ libc-dev musl-dev

WORKDIR /opt/pyroscope

ENV ENABLED_SPIES none

COPY --from=js-builder /opt/pyroscope/webapp/public ./webapp/public
COPY pkg ./pkg
COPY cmd ./cmd
COPY tools ./tools
COPY scripts ./scripts
COPY go.mod go.sum pyroscope.go ./
COPY Makefile ./

RUN EMBEDDED_ASSETS_DEPS="" make build-release

# final image

FROM alpine:3.12

LABEL maintainer="Pyroscope team <hello@pyroscope.io>"

WORKDIR $PS_PATHS_HOME

RUN apk add --no-cache ca-certificates bash tzdata && \
    apk add --no-cache openssl musl-utils

RUN addgroup -S pyroscope && adduser -S pyroscope -G pyroscope

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
