################################
# Build Image
################################
FROM golang:1.22-bookworm AS build

WORKDIR /go/src/app

# copy source directories in the order they are least likely to most likely to change, to improve cache utilization
COPY assert assert
COPY e2e/testgames e2e/testgames
COPY cardinal cardinal
COPY rift rift
COPY sign sign

# cardinal and assert are part of the go mods, so this must be done after they are copied
WORKDIR /go/src/app/e2e/testgames
RUN go mod download

# build the binary with debugging symbols
WORKDIR /go/src/app/e2e/testgames/game
RUN go build -v -o /go/bin/app


################################
# Runtime Image - Normal
################################
FROM gcr.io/distroless/base-debian12 AS runtime

# Copy the binary from the build image
COPY --from=build /go/bin/app /usr/bin

# Run the binary
CMD ["app"]

################################
# Runtime Image - Debug
################################
FROM golang:1.22-bookworm AS runtime-debug

# Install delve
RUN go install github.com/go-delve/delve/cmd/dlv@latest

# Set the GOCACHE environment variable to /root/.cache/go-build to speed up build
ENV GOCACHE=/root/.cache/go-build

WORKDIR /go/src/app

# copy source directories in the order they are least likely to most likely to change, to improve cache utilization
COPY assert assert
COPY e2e/testgames e2e/testgames
COPY cardinal cardinal
COPY rift rift
COPY sign sign

# cardinal and assert are part of the go mods, so this must be done after they are copied
WORKDIR /go/src/app/e2e/testgames
RUN go mod download

# build the binary with debugging symbols
WORKDIR /go/src/app/e2e/testgames/game
RUN --mount=type=cache,target="/root/.cache/go-build" go build -gcflags="all=-N -l" -v -o /usr/bin/app

# Run the binary under the Delve debugger, with it listening for remote debug connections
CMD ["dlv", "--listen=:40000", "--headless=true", "--api-version=2", "--accept-multiclient", "exec", "/usr/bin/app"]
