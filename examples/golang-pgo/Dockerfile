FROM golang:1.26.8

WORKDIR /go/src/app
COPY . .

RUN go build main.go

CMD ["./main"]
