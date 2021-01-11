FROM golang:1.15.6

WORKDIR /go/src/app

COPY main.go ./main.go

RUN go get -d ./
RUN go build -o main .
CMD ["./main"]
