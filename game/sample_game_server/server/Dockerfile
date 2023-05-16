FROM golang:1.20

WORKDIR /usr/src/app

copy go.mod .
copy go.sum .
copy *.go .
copy vendor vendor/

RUN go build -v -o /usr/local/bin/app ./...

CMD ["app"]


