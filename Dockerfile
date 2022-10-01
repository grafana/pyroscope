#        _
#       | |
#  _ __ | |__  _ __  ___ _ __  _   _
# | '_ \| '_ \| '_ \/ __| '_ \| | | |
# | |_) | | | | |_) \__ \ |_) | |_| |
# | .__/|_| |_| .__/|___/ .__/ \__, |
# | |         | |       | |     __/ |
# |_|         |_|       |_|    |___/

FROM php:7.3-fpm-alpine3.13 as phpspy-builder
RUN apk add --update alpine-sdk
COPY Makefile Makefile
RUN mkdir -p third_party/phpspy
RUN make build-phpspy-dependencies

#                     _
#                    | |
#   __ _ ___ ___  ___| |_ ___
#  / _` / __/ __|/ _ \ __/ __|
# | (_| \__ \__ \  __/ |_\__ \
#  \__,_|___/___/\___|\__|___/

FROM node:14.17.6-alpine3.12 as js-builder

RUN apk add --no-cache make

WORKDIR /opt/pyroscope

COPY scripts ./scripts
COPY package.json yarn.lock Makefile lerna.json ./
COPY lib ./lib
COPY packages ./packages
COPY babel.config.js .eslintrc.js .eslintignore .prettierrc tsconfig.json ./
COPY webapp ./webapp

# we only need the dependencies required to BUILD the application
RUN --mount=type=cache,target=/usr/local/share/.cache/yarn/v6 make install-build-web-dependencies


ARG EXTRA_METADATA=""

RUN EXTRA_METADATA=$EXTRA_METADATA make assets-release



#       _            __
#      | |          / _|
#   ___| |__  _ __ | |_
#  / _ \ '_ \| '_ \|  _|
# |  __/ |_) | |_) | |
#  \___|_.__/| .__/|_|
#            | |
#            |_|
FROM alpine:3.12 as ebpf-builder
RUN apk add cmake make binutils gcc g++ clang musl-dev linux-headers zlib-dev elfutils-dev libelf-static zlib-static git openssh
ADD third_party/libbpf/Makefile /build/libbpf/
RUN make -C /build/libbpf/
ADD third_party/bcc/Makefile /build/bcc/
RUN make -C /build/bcc/
ADD pkg/agent/ebpfspy/bpf/Makefile pkg/agent/ebpfspy/bpf/profile.bpf.c pkg/agent/ebpfspy/bpf/profile.bpf.h /build/profile.bpf/
RUN CFLAGS=-I/build/libbpf/lib/include make -C /build/profile.bpf

#              _
#             | |
#   __ _  ___ | | __ _ _ __   __ _
#  / _` |/ _ \| |/ _` | '_ \ / _` |
# | (_| | (_) | | (_| | | | | (_| |
#  \__, |\___/|_|\__,_|_| |_|\__, |
#   __/ |                     __/ |
#  |___/                     |___/

# We build our own golang image because we need alpine 3.12 and go 1.17 is not available in alpine 3.12
# The dockerfile we use is a copy of this one:
#   https://github.com/docker-library/golang/blob/48e32c58a6abc052253fba899cea876740cab262/1.16/alpine3.14/Dockerfile
# TODO: figure out why linking isn't working on alpine 3.13 or 3.14
# see https://github.com/pyroscope-io/pyroscope/pull/372 for more context
FROM pyroscope/golang:1.18.0-alpine3.12 AS go-builder

RUN apk add --no-cache make git zstd gcc g++ libc-dev musl-dev bash zlib-dev elfutils-dev libelf-static zlib-static \
    linux-headers
RUN apk upgrade binutils
RUN apk upgrade elfutils

WORKDIR /opt/pyroscope


COPY third_party/phpspy/phpspy.h /opt/pyroscope/third_party/phpspy/phpspy.h
COPY --from=phpspy-builder /var/www/html/third_party/phpspy/libphpspy.a /opt/pyroscope/third_party/phpspy/libphpspy.a
COPY --from=js-builder /opt/pyroscope/webapp/public ./webapp/public
COPY --from=ebpf-builder /build/bcc/lib third_party/bcc/lib
COPY --from=ebpf-builder /build/libbpf/lib third_party/libbpf/lib
COPY --from=ebpf-builder /build/profile.bpf/profile.bpf.o pkg/agent/ebpfspy/bpf/profile.bpf.o
COPY Makefile ./
COPY tools ./tools
COPY go.mod go.sum ./
RUN make install-dev-tools
RUN make install-go-dependencies

COPY pkg ./pkg
COPY cmd ./cmd
COPY webapp/assets_embedded.go ./webapp/assets_embedded.go
COPY webapp/assets.go ./webapp/assets.go
COPY scripts ./scripts

RUN ENABLED_SPIES_RELEASE="ebpfspy,phpspy,dotnetspy" \
    EMBEDDED_ASSETS_DEPS="" \
    EXTRA_LDFLAGS="-linkmode external -extldflags '-static'" \
    make build-release



#   __ _             _   _
#  / _(_)           | | (_)
# | |_ _ _ __   __ _| |  _ _ __ ___   __ _  __ _  ___
# |  _| | '_ \ / _` | | | | '_ ` _ \ / _` |/ _` |/ _ \
# | | | | | | | (_| | | | | | | | | | (_| | (_| |  __/
# |_| |_|_| |_|\__,_|_| |_|_| |_| |_|\__,_|\__, |\___|
#                                           __/ |
#                                          |___/

FROM alpine:3.12

LABEL maintainer="Pyroscope team <hello@pyroscope.io>"

WORKDIR /var/lib/pyroscope

RUN apk add --no-cache ca-certificates bash tzdata openssl musl-utils

RUN ln -s $(which python3) /usr/bin/python

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
COPY --from=go-builder --chmod=0777 /opt/pyroscope/bin/pyroscope /usr/bin/pyroscope
# we use this in cloud
COPY --from=js-builder /opt/pyroscope/webapp/public/standalone.html /standalone.html

RUN apk add bash-completion
RUN pyroscope completion bash > /usr/share/bash-completion/completions/pyroscope

USER pyroscope
EXPOSE 4040/tcp
ENTRYPOINT [ "/usr/bin/pyroscope" ]
