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
# Runtime Image
################################
FROM gcr.io/distroless/base-debian12 AS runtime

# Copy the binary from the build image
COPY --from=build /go/bin/app /usr/bin

# Run the binary
CMD ["app"]