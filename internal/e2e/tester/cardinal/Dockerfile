FROM golang:1.21

WORKDIR /usr/src/app

COPY . .
COPY vendor vendor/

RUN go build -v -o /usr/local/bin/app

EXPOSE 4040 9020

CMD ["app"]