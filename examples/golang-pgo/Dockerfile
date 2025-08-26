FROM golang:1.23.12

WORKDIR /go/src/app
COPY . .

RUN go build main.go

CMD ["./main"]
