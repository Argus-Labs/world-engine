################################
# Build Image
################################
FROM golang:1.22-bookworm AS build

WORKDIR /go/src/app

COPY e2e/testgames e2e/testgames
COPY cardinal cardinal
COPY assert assert

# Copy the go module files and download the dependencies
# We do this before copying the rest of the source code to avoid
# having to re-download the dependencies every time we build the image
RUN cd e2e/testgames && go mod download
RUN cd e2e/testgames/game && go build -v -o /go/bin/app


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

# assert seems like it will be least likely to change
COPY assert assert

# we assume here that testgames will rarely change
COPY e2e/testgames e2e/testgames

# assumes we are most often testing changes to cardinal code
COPY cardinal cardinal

# cardinal and assert are part of the go mods, so this must be done after they are copied
RUN cd e2e/testgames && go mod download

# build the binary with debugging symbols
RUN --mount=type=cache,target="/root/.cache/go-build" cd e2e/testgames/game && go build -gcflags="all=-N -l" -v -o /usr/bin/app

CMD ["dlv", "--listen=:40000", "--headless=true", "--api-version=2", "--accept-multiclient", "exec", "/usr/bin/app"]