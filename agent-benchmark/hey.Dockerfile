FROM golang:1.16 AS builder

RUN git clone https://github.com/rakyll/hey.git
WORKDIR hey
RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /hey

FROM ubuntu:latest

COPY --from=builder /hey /usr/local/bin/hey

ENTRYPOINT ["/hey"]
