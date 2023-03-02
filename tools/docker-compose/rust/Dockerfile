FROM rust:1.64.0-bullseye as build

WORKDIR /usr/src/cp-rust
RUN cargo init
COPY Cargo.toml Cargo.lock ./

RUN \
  cargo build && \
  rm -rf ./src

COPY src ./src/

RUN cargo install --debug --path .

CMD ["cp-rust"]

FROM debian:bullseye-slim

COPY --from=build /usr/src/cp-rust/target/debug/cp-rust /usr/bin/cp-rust

ENV RUST_LOG=debug
ENV RUST_BACKTRACE=full

EXPOSE 8080

CMD ["cp-rust"]
