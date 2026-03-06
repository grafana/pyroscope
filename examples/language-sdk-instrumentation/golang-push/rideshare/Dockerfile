FROM golang:1.25.7

WORKDIR /go/src/app
COPY . .
RUN go build main.go
CMD ["./main"]
