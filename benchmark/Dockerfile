FROM golang:1.15.6

WORKDIR /go/src/app

COPY go.mod go.sum ./
RUN go mod download

COPY ./pkg pkg
COPY ./benchmark benchmark

RUN go build -o benchmark-main ./benchmark
CMD ["./benchmark-main"]
