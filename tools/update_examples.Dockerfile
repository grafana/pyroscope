
ARG NODE_VERSION=node:18

FROM ${NODE_VERSION} AS node


FROM ubuntu:22.04

RUN apt-get update && apt-get -y install wget git build-essential libssl-dev libgmp-dev libyaml-dev zlib1g-dev curl \
    gawk autoconf automake bison libffi-dev libgdbm-dev libsqlite3-dev libtool pkg-config sqlite3 libncurses5-dev \
    libreadline-dev gnupg

ARG GO_VERSION=1.22.10
RUN wget https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz
ENV PATH=$PATH:/usr/local/go/bin



ARG GH_VERSION=2.63.2
RUN wget https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_amd64.deb && \
    dpkg -i gh_${GH_VERSION}_linux_amd64.deb

COPY --from=node /usr/local/lib /usr/local/lib
COPY --from=node /usr/local/bin /usr/local/bin
COPY --from=node /opt /opt


ENV PATH=$PATH:/usr/local/bin

ARG RUBY_VERSION=3.2.2
RUN curl -sSL https://get.rvm.io | bash
RUN /bin/bash -l -c "rvm install ruby-${RUBY_VERSION} && rvm --default use ruby-${RUBY_VERSION}"

RUN curl https://sh.rustup.rs -sSf | sh -s -- -y --default-toolchain stable
ENV PATH=$PATH:/root/.cargo/bin