FROM golang:1.19 as builder

WORKDIR /src/chain

COPY chain/go.mod .
COPY chain/go.sum .

COPY chain .

WORKDIR /src/rollup

COPY rollup/go.mod .
COPY rollup/go.sum .

RUN go mod download

COPY rollup .

FROM golang:1.19

WORKDIR /root

COPY --from=builder /src/rollup .
COPY --from=builder /src/chain .

CMD go run cmd/test/main.go
