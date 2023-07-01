FROM golang:1.20

WORKDIR /usr/src/app

copy . .
copy vendor vendor/

RUN go build -v -o /usr/local/bin/app

CMD ["app"]


