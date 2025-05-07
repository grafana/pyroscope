FROM golang:1.23.8

WORKDIR /go/src/app
COPY . .
RUN go build main.go
CMD ["./main"]
