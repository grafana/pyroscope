#                        _                     _
#                       | |                   | |
#  _ __  _   _ _ __ ___ | |__   ___ _ __   ___| |__
# | '_ \| | | | '__/ _ \| '_ \ / _ \ '_ \ / __| '_ \
# | |_) | |_| | | | (_) | |_) |  __/ | | | (__| | | |
# | .__/ \__, |_|  \___/|_.__/ \___|_| |_|\___|_| |_|
# | |     __/ |
# |_|    |___/
#

FROM golang:1.16.3 as builder

WORKDIR /go/src/app

COPY go.mod go.sum ./
RUN go mod download

COPY ./pkg pkg
COPY ./webapp webapp
COPY ./benchmark benchmark

RUN go build -o pyrobench ./benchmark/cmd

COPY ./benchmark/internal/server/fixtures fixtures

USER pyrobench
CMD ["./pyrobench"]

FROM ubuntu:latest

WORKDIR /var/lib/pyrobench

RUN apt-get update && apt-get install ca-certificates -y && update-ca-certificates
# Create a group and user
#RUN addgroup -S pyrobench && adduser -S pyrobench -G pyrobench
RUN useradd -ms /bin/bash pyrobench


COPY --from=builder /go/src/app/pyrobench pyrobench

USER pyrobench
CMD ["./pyrobench"]
