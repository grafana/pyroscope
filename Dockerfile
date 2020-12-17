# assets build

FROM --platform=$BUILDPLATFORM node:14.15.1-alpine3.12 as js-builder

RUN apk add --no-cache make

WORKDIR /opt/pyroscope

COPY package.json package-lock.json .babelrc Makefile ./
COPY scripts ./scripts
COPY webapp ./webapp

# RUN make assets
RUN echo 1

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

ARG PS_UID="472"
ARG PS_GID="0"

ENV PATH="/opt/pyroscope/bin:$PATH" \
    PS_PATHS_CONFIG="/etc/pyroscope/" \
    PS_PATHS_DATA="/var/lib/pyroscope" \
    PS_PATHS_HOME="/opt/pyroscope" \
    PS_PATHS_LOGS="/var/log/pyroscope"

WORKDIR $PS_PATHS_HOME

RUN apk add --no-cache ca-certificates bash tzdata && \
    apk add --no-cache openssl musl-utils

COPY conf ./conf

RUN if [ ! $(getent group "$PS_GID") ]; then \
      addgroup -S -g $PS_GID pyroscope; \
    fi

RUN export PS_GID_NAME=$(getent group $PS_GID | cut -d':' -f1) && \
    mkdir -p "$PS_PATHS_HOME/.aws" && \
    adduser -S -u $PS_UID -G "$PS_GID_NAME" pyroscope && \
    mkdir -p "$PS_PATHS_LOGS" \
             "$PS_PATHS_CONFIG" \
             "$PS_PATHS_DATA" \
             && \
    mv "$PS_PATHS_HOME/conf/server.yml" "$PS_PATHS_CONFIG" && \
    chown -R "pyroscope:$PS_GID_NAME" \
        "$PS_PATHS_DATA" \
        "$PS_PATHS_HOME" \
        "$PS_PATHS_LOGS" \
        "$PS_PATHS_CONFIG" \
        && \
    chmod -R 777 "$PS_PATHS_DATA" "$PS_PATHS_HOME" "$PS_PATHS_LOGS" "$PS_PATHS_CONFIG"

COPY --from=go-builder /opt/pyroscope/bin/pyroscope ./bin/pyroscope

USER pyroscope
EXPOSE 8080/tcp
ENTRYPOINT [ "/opt/pyroscope/bin/pyroscope", "server" ]
