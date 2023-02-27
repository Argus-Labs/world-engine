FROM golang:1.19 as builder

WORKDIR /src/rollup

COPY rollup/cmd/test .
RUN go install

FROM scratch

COPY --from=builder /src/rollup .

RUN
