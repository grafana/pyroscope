FROM golang:1.25.13

WORKDIR /go/src/app
COPY . .

RUN go build main.go

CMD ["./main"]
