FROM golang:1.17

WORKDIR /go/src/app
COPY . .

RUN echo $(pwd)

RUN go get -d -v ./...
RUN go install -v ./...

CMD ["go", "run", "."]
