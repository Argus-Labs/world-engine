ARG IMG_TAG=latest

# Compile the gaiad binary
FROM golang:1.19-alpine AS go-builder
WORKDIR /src/app/
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ENV PACKAGES curl make git libc-dev bash gcc linux-headers eudev-dev python3
RUN apk add --no-cache $PACKAGES
RUN CGO_ENABLED=0 make install


FROM alpine:3.12
COPY --from=go-builder /go/bin/gaiad /usr/local/bin/
EXPOSE 26656 26657 1317 9090
USER 0

COPY contrib/single-node.sh single-node.sh
RUN chmod +x single-node.sh

ENTRYPOINT ["./single-node.sh"]
