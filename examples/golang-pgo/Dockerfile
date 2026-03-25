FROM golang:1.25.8

WORKDIR /go/src/app
COPY . .

RUN go build main.go

CMD ["./main"]
