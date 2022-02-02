FROM rust:latest

WORKDIR /usr/src/server
COPY server/ .
RUN cargo install --path .
EXPOSE 5000
CMD ["server"]
