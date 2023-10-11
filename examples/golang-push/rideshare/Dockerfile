FROM golang:1.21.3

WORKDIR /go/src/app
COPY . .
RUN go build main.go
CMD ["./main"]
