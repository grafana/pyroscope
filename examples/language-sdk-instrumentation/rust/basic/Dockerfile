FROM rust:latest

WORKDIR /usr/src/server
COPY src src
COPY Cargo.toml Cargo.lock ./
RUN cargo install --profile release --path .
EXPOSE 5000
CMD ["basic"]
