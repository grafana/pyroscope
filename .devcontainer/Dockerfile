FROM mcr.microsoft.com/vscode/devcontainers/go:1.17

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
RUN npm config set unsafe-perm true
RUN npm install -g yarn

# libunwind
RUN apt-get update && export DEBIAN_FRONTEND=noninteractive \
    && apt-get -y install --no-install-recommends libunwind8-dev

WORKDIR /workspaces/pyroscope

USER vscode
