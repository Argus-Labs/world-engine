ARG IMG_TAG=latest

# Compile the argus binary
FROM golang:1.18-alpine AS argusd-builder
WORKDIR /src/app/
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ENV PACKAGES curl make git libc-dev bash gcc linux-headers eudev-dev python3
RUN apk add --no-cache $PACKAGES
RUN CGO_ENABLED=0 make install

# Add to a distroless container
FROM distroless.dev/static:$IMG_TAG
ARG IMG_TAG
COPY --from=argusd-builder /go/bin/argusd /usr/local/bin/
EXPOSE 26656 26657 1317 9090

ENTRYPOINT ["argusd", "start"]
