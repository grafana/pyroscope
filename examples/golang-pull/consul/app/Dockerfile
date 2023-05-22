FROM golang:1.18.3

WORKDIR /go/src/app

COPY main.go ./main.go
COPY go.mod ./go.mod
COPY go.sum ./go.sum

RUN go build -o main .

RUN adduser --disabled-password --gecos --quiet pyroscope
USER pyroscope

CMD ["./main"]
