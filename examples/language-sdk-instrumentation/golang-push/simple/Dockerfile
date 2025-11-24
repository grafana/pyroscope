FROM golang:1.24.10

WORKDIR /go/src/app

COPY main.go go.mod go.sum ./

RUN go get -d ./
RUN go build -o main .

RUN adduser --disabled-password --gecos --quiet pyroscope
USER pyroscope

CMD ["./main"]
