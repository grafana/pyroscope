FROM mcr.microsoft.com/vscode/devcontainers/go:1.15

USER root

# Rust
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
RUN /root/.cargo/bin/rustup target add $(uname -m)-unknown-linux-musl

# PHP
RUN apt-get update && export DEBIAN_FRONTEND=noninteractive \
    && apt-get -y install --no-install-recommends php php-dev

# Node
RUN apt-get update && export DEBIAN_FRONTEND=noninteractive \
    && apt-get -y install --no-install-recommends nodejs npm
RUN npm install -g yarn

# libunwind
RUN wget https://github.com/libunwind/libunwind/releases/download/v1.3.1/libunwind-1.3.1.tar.gz && \
    tar -zxf libunwind-1.3.1.tar.gz && \
    cd libunwind-1.3.1/ && \
    ./configure --disable-minidebuginfo --enable-ptrace --disable-tests --disable-documentation && make && make install

WORKDIR /workspaces/pyroscope

USER vscode
