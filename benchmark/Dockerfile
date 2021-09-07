FROM golang:1.16.3 as builder

WORKDIR /go/src/app

COPY go.mod go.sum ./
RUN go mod download

COPY ./pkg pkg
COPY ./webapp webapp
COPY ./benchmark benchmark

RUN go build -o pyrobench ./benchmark/cmd

USER pyrobench
CMD ["./pyrobench"]

FROM ubuntu:latest

WORKDIR /var/lib/pyrobench
# Create a group and user
#RUN addgroup -S pyrobench && adduser -S pyrobench -G pyrobench
RUN useradd -ms /bin/bash pyrobench


COPY --from=builder /go/src/app/pyrobench pyrobench

USER pyrobench
CMD ["./pyrobench"]
