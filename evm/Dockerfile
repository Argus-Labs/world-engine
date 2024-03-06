# Compile the argus binary
FROM golang:1.22.1-bullseye AS argusd-builder
WORKDIR /src/app/
COPY go.mod go.sum* ./
COPY . .
RUN go mod download
ENV PACKAGES curl make git libc-dev bash gcc linux-headers eudev-dev python3
RUN CGO_ENABLED=1 make install

# build on ubuntu
FROM ubuntu:18.04
COPY --from=argusd-builder /go/bin/world-evm /usr/local/bin/
EXPOSE 26656 26657 1317 9090 8546 8545 9601

USER 0

COPY scripts/start-sequencer.sh start-sequencer.sh
RUN chmod +x start-sequencer.sh
COPY scripts/app.toml app.toml

HEALTHCHECK --interval=5s --timeout=80s CMD curl --fail http://localhost:26657 || exit 1

ENTRYPOINT ["./start-sequencer.sh"]
