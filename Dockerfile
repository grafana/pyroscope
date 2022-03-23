#                 _
#                | |
#  _ __ _   _ ___| |_
# | '__| | | / __| __|
# | |  | |_| \__ \ |_
# |_|   \__,_|___/\__|

FROM alpine:3.12 as rust-builder

RUN apk update &&\
    apk add --no-cache git gcc g++ make build-base openssl-dev musl musl-dev curl zlib-static

RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
RUN /root/.cargo/bin/rustup target add $(uname -m)-unknown-linux-musl

RUN wget https://github.com/libunwind/libunwind/releases/download/v1.3.1/libunwind-1.3.1.tar.gz && \
    tar -zxf libunwind-1.3.1.tar.gz && \
    cd libunwind-1.3.1/ && \
    ./configure --with-pic --disable-minidebuginfo --enable-ptrace --disable-tests --disable-documentation && make && make install

COPY third_party/rustdeps /opt/rustdeps

WORKDIR /opt/rustdeps

RUN RUSTFLAGS="-L /lib -C target-feature=+crt-static" /root/.cargo/bin/cargo build --release --target $(uname -m)-unknown-linux-musl
RUN mv /opt/rustdeps/target/$(uname -m)-unknown-linux-musl/release/librustdeps.a /opt/rustdeps/librustdeps.a

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
FROM pyroscope/golang:1.17.0-alpine3.12 AS go-builder

RUN apk add --no-cache make git zstd gcc g++ libc-dev musl-dev bash
RUN apk upgrade binutils
RUN apk upgrade elfutils

WORKDIR /opt/pyroscope

RUN mkdir -p /opt/pyroscope/third_party/rustdeps/target/release
COPY --from=rust-builder /opt/rustdeps/librustdeps.a /opt/pyroscope/third_party/rustdeps/target/release/librustdeps.a
COPY third_party/rustdeps/rbspy.h /opt/pyroscope/third_party/rustdeps/rbspy.h
COPY third_party/rustdeps/pyspy.h /opt/pyroscope/third_party/rustdeps/pyspy.h
COPY third_party/phpspy/phpspy.h /opt/pyroscope/third_party/phpspy/phpspy.h
COPY --from=phpspy-builder /var/www/html/third_party/phpspy/libphpspy.a /opt/pyroscope/third_party/phpspy/libphpspy.a
COPY --from=js-builder /opt/pyroscope/webapp/public ./webapp/public
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

# Alpine's default stack size too small for pyspy, causing exec mode with pyspy to segfault.
# See https://github.com/pyroscope-io/pyroscope/issues/503
RUN EMBEDDED_ASSETS_DEPS="" \
    CGO_LDFLAGS_ALLOW="-Wl,-z,stack-size=0x200000" \
    EXTRA_LDFLAGS="-linkmode external -extldflags '-static -Wl,-z,stack-size=0x200000'" \
    make build-release

#      _        _   _        _ _ _
#     | |      | | (_)      | (_) |
#  ___| |_ __ _| |_ _  ___  | |_| |__  ___
# / __| __/ _` | __| |/ __| | | | '_ \/ __|
# \__ \ || (_| | |_| | (__  | | | |_) \__ \
# |___/\__\__,_|\__|_|\___| |_|_|_.__/|___/


FROM go-builder AS go-libs-builder

RUN make build-rbspy-static-library
RUN make build-pyspy-static-library
RUN make build-phpspy-static-library

FROM scratch AS lib-exporter

COPY --from=go-libs-builder /opt/pyroscope/out/libpyroscope.phpspy.a /
COPY --from=go-libs-builder /opt/pyroscope/third_party/phpspy/libphpspy.a /
COPY --from=go-libs-builder /opt/pyroscope/out/libpyroscope.phpspy.h /
COPY --from=go-libs-builder /opt/pyroscope/out/libpyroscope.pyspy.a /
COPY --from=go-libs-builder /opt/pyroscope/out/libpyroscope.pyspy.h /
COPY --from=go-libs-builder /opt/pyroscope/out/libpyroscope.rbspy.a /
COPY --from=go-libs-builder /opt/pyroscope/out/libpyroscope.rbspy.h /
COPY --from=rust-builder /opt/rustdeps/librustdeps.a /


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
RUN apk add --no-cache bcc-tools python3
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

RUN apk add bash-completion
RUN pyroscope completion bash > /usr/share/bash-completion/completions/pyroscope

USER pyroscope
EXPOSE 4040/tcp
ENTRYPOINT [ "/usr/bin/pyroscope" ]
