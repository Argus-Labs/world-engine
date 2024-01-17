FROM golang:1.21 AS builder

WORKDIR /usr/src/app

copy . .

RUN go build -v -o /usr/local/bin/app

FROM ubuntu:22.04
COPY --from=builder /usr/local/bin/app /usr/local/bin/

CMD ["app"]
