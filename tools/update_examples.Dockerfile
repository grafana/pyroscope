
ARG NODE_VERSION=node:26.12.1@sha256:97fa0f1b59035c6072721a88d55be1371c7cb1cc942b521e1cc5c72ec87655f4

FROM ${NODE_VERSION} AS node


FROM ubuntu:26.04@sha256:c098726dd9b9733f3e885f8de0ef375206a311b0a317eadc40d347b1115fb944

RUN apt-get update && apt-get -y install wget git build-essential libssl-dev libgmp-dev libyaml-dev zlib1g-dev curl \
    gawk autoconf automake bison libffi-dev libgdbm-dev libsqlite3-dev libtool pkg-config sqlite3 libncurses5-dev \
    libreadline-dev gnupg

ARG TARGETARCH
ARG GO_VERSION=1.26.10
RUN wget https://go.dev/dl/go${GO_VERSION}.linux-${TARGETARCH}.tar.gz && \
    tar -C /usr/local -xzf go${GO_VERSION}.linux-${TARGETARCH}.tar.gz
ENV PATH=$PATH:/usr/local/go/bin



ARG GH_VERSION=2.103.2
RUN wget https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_${TARGETARCH}.deb && \
    dpkg -i gh_${GH_VERSION}_linux_${TARGETARCH}.deb

COPY --from=node /usr/local/lib /usr/local/lib
COPY --from=node /usr/local/bin /usr/local/bin
COPY --from=node /opt /opt


ENV PATH=$PATH:/usr/local/bin

# Node 26 no longer ships Yarn; these examples use Classic lockfiles.
ARG YARN_VERSION=1.22.22
RUN npm install --global yarn@${YARN_VERSION}

# Keep aligned with the examples; Ruby 3.3.13 requires pyroscope >= 1.1.7.
ARG RUBY_VERSION=3.3.13
RUN curl -sSL https://get.rvm.io | bash
RUN /bin/bash -l -c "rvm install ruby-${RUBY_VERSION} && rvm --default use ruby-${RUBY_VERSION}"

RUN curl https://sh.rustup.rs -sSf | sh -s -- -y --default-toolchain stable
ENV PATH=$PATH:/root/.cargo/bin

# The repo is bind-mounted at /pyroscope (see Makefile.examples) and owned by
# the runner user, while this image runs as root, so git refuses to operate on
# it until the path is trusted.
RUN git config --global --add safe.directory /pyroscope