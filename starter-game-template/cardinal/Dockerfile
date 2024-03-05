# Base Builder Stage
FROM golang:1.22 AS builder

RUN go install github.com/go-delve/delve/cmd/dlv@latest

WORKDIR /usr/src/app
COPY . .

RUN go build -v -o /usr/local/bin/app
RUN go build -gcflags="all=-N -l" -v -o /usr/local/bin/app-debug

# Runtime Debug Stage
FROM ubuntu:22.04 AS runtime-debug

COPY --from=builder /go/bin/dlv /usr/local/bin
COPY --from=builder /usr/local/bin/app-debug /usr/local/bin/

CMD ["dlv", "--listen=:40000", "--headless=true", "--api-version=2", "--accept-multiclient", "exec", "/usr/local/bin/app-debug"]


# Runtime Stage
FROM ubuntu:22.04 AS runtime

COPY --from=builder /usr/local/bin/app /usr/local/bin/

CMD ["app"]
