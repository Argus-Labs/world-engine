# Compile the argus binary
FROM golang:1.19-bullseye AS argusd-builder
WORKDIR /src/app/
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ENV PACKAGES curl make git libc-dev bash gcc linux-headers eudev-dev python3
#RUN apk add --no-cache $PACKAGES
RUN CGO_ENABLED=1 make install


# Add to a distroless container
FROM ubuntu:18.04
COPY --from=argusd-builder /go/bin/argusd /usr/local/bin/
EXPOSE 26656 26657 1317 9090
USER 0

COPY contrib/single-node.sh single-node.sh
RUN chmod +x single-node.sh

HEALTHCHECK --interval=5s --timeout=80s CMD curl --fail http://localhost:26657 || exit 1

ENTRYPOINT ["./single-node.sh"]
