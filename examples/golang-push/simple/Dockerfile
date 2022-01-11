FROM golang:1.15.6

WORKDIR /go/src/app

COPY main.go ./main.go

RUN go get -d ./
RUN go build -o main .

RUN adduser --disabled-password --gecos --quiet pyroscope
USER pyroscope

CMD ["./main"]
