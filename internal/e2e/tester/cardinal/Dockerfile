FROM golang:1.20

WORKDIR /usr/src/app

COPY . .
COPY vendor vendor/

RUN go build -v -o /usr/local/bin/app

EXPOSE 4040 9020

CMD ["app"]