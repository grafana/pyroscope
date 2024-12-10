FROM golang:1.22.10

WORKDIR /go/src/app
COPY . .

RUN go build main.go

CMD ["./main"]
