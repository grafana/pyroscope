FROM golang:1.21.3

WORKDIR /go/src/app

COPY main.go ./main.go

RUN go mod init github.com/pyroscope-io/pyroscope/examples/golang-pull/static
RUN go get -d ./
RUN go build -o main .

RUN adduser --disabled-password --gecos --quiet pyroscope
USER pyroscope

CMD ["./main"]
