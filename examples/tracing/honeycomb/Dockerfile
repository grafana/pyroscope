FROM golang:1.17

WORKDIR /go/src/app

COPY . .

RUN go get -d ./
RUN go build -o main main.go

RUN adduser --disabled-password --gecos --quiet pyroscope
USER pyroscope

CMD ["./main"]
