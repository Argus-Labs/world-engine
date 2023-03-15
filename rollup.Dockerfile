FROM golang:1.19 as builder

WORKDIR /src/chain

COPY chain/go.mod .
COPY chain/go.sum .

RUN go mod download

COPY chain/ .

WORKDIR /src/rollup

COPY rollup/ .

RUN go mod download

COPY rollup/cmd/test /cmd/test

WORKDIR /src/rollup/cmd/test

RUN go build

RUN chmod +x start.sh


CMD ["./start.sh"]
