FROM golang:1.22

WORKDIR /go/src/app

COPY evm evm
COPY cardinal cardinal
COPY rift rift

# Copy the go module files and download the dependencies
# We do this before copying the rest of the source code to avoid
# having to re-download the dependencies every time we build the image
COPY e2e/tests/go.mod e2e/tests/go.sum e2e/tests/
RUN cd e2e/tests && go mod download

# Copy the rest of the source code and build the binary
COPY e2e/tests e2e/tests
ENTRYPOINT cd /go/src/app/e2e/tests/nakama && go test -v --tags integration
