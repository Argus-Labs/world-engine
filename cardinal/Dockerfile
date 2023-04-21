FROM golang:1.19-bullseye AS builder
WORKDIR /src/app/

COPY go.mod go.sum* ./
RUN go mod download
COPY . .

WORKDIR /src/app/cmd/test

RUN go build

RUN chmod +x start.sh

CMD ["./start.sh"]
