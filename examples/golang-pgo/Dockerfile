FROM golang:1.24.13

WORKDIR /go/src/app
COPY . .

RUN go build main.go

CMD ["./main"]
