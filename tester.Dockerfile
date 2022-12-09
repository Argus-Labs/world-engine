FROM golang:1.19 as builder

WORKDIR /src/app

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

FROM golang:1.19
WORKDIR /root

COPY --from=builder /src/app .

CMD go test ./...tests/e2e/sidecar
