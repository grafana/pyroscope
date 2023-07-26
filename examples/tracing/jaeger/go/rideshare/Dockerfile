FROM golang:1.17

WORKDIR /go/src/app

COPY go.mod go.sum ./
RUN go mod download

COPY bike bike
COPY car car
COPY log log
COPY ride ride
COPY rideshare rideshare
COPY scooter scooter
COPY main.go ./
RUN go build -o main main.go

RUN adduser --disabled-password --gecos --quiet pyroscope
USER pyroscope

CMD ["./main"]
