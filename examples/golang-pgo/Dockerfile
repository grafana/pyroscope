FROM golang:1.21.11

WORKDIR /go/src/app
COPY . .

RUN go build main.go

CMD ["./main"]
