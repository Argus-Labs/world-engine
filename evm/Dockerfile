################################
# Build Image
################################
FROM --platform=$TARGETPLATFORM golang:1.22.1-bullseye AS build

ARG TARGETPLATFORM
ARG BUILDPLATFORM

WORKDIR /go/src/app/

# Copy the go module files and download the dependencies
# We do this before copying the rest of the source code to avoid
# having to re-download the dependencies every time we build the image
COPY evm evm
COPY rift rift

WORKDIR /go/src/app/evm
RUN go mod download

# build the binary
ENV PACKAGES="curl make git libc-dev bash gcc linux-headers eudev-dev python3"
RUN CGO_ENABLED=1 make install # Binary will be in /go/bin/world-evm

################################
# Runtime Image
################################
FROM --platform=$TARGETPLATFORM ubuntu:18.04 AS runtime

# Expose ports needed by the World EVM
EXPOSE 26656 26657 1317 9090 8546 8545 9601

# Install package dependencies for start script
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update -y &&\
 apt-get install -y --no-install-recommends curl jq bash && \
 apt-get autoremove -y && \
 apt-get clean -y && \
 rm -rf /tmp/* /var/tmp/* /var/cache/apt/archives/* /var/lib/apt/lists/*

# Copy the World EVM binary, start script, and configuration files
COPY --from=build /go/bin/world-evm /usr/local/bin/
COPY evm/scripts/start-sequencer.sh evm/scripts/app.toml evm/scripts/config.toml ./

# Start the World EVM sequencer
USER 0
RUN chmod +x start-sequencer.sh
ENTRYPOINT ["./start-sequencer.sh"]
