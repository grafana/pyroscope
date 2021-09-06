FROM golang:1.16.3

WORKDIR /go/src/app

COPY go.mod go.sum ./
RUN go mod download

COPY ./pkg pkg
COPY ./webapp webapp
COPY ./benchmark benchmark

RUN go build -o pyrobench ./benchmark/cmd
CMD ["./pyrobench"]
