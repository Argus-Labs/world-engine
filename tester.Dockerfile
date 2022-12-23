FROM golang:1.19 as builder

WORKDIR /src/app

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

FROM golang:1.19

ARG TEST_PACKAGE
ENV TEST_PACKAGE=$TEST_PACKAGE

WORKDIR /root

COPY --from=builder /src/app .

CMD go test $TEST_PACKAGE
