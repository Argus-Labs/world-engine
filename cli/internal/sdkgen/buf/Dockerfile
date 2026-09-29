# buf + pinned plugins for `world sdk generate`. Plugins are LOCAL (no buf.build
# account, no BSR calls). Versions come in as build ARGs (world-cli passes the
# embedded defaults from proto/buf.gen.yaml, or the --go-version/--csharp-version
# overrides). Generated proto has no services, so only message plugins are needed:
# protoc-gen-go (Go) and protoc's built-in C# backend.
ARG GO_PLUGIN_VERSION=v1.36.11
ARG PROTOC_VERSION=33.2

FROM golang:1.27-bookworm AS gobuild
ARG GO_PLUGIN_VERSION
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@${GO_PLUGIN_VERSION}

FROM debian:bookworm-slim
ARG PROTOC_VERSION
ARG TARGETARCH
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl unzip \
 && rm -rf /var/lib/apt/lists/*

# buf 1.66.0 (matches .prototools)
RUN case "$TARGETARCH" in amd64) A=x86_64 ;; arm64) A=aarch64 ;; *) A=x86_64 ;; esac \
 && curl -fsSL -o /usr/local/bin/buf "https://github.com/bufbuild/buf/releases/download/v1.66.0/buf-Linux-$A" \
 && chmod +x /usr/local/bin/buf

# protoc ${PROTOC_VERSION} (built-in C# backend, matches protocolbuffers/csharp)
RUN case "$TARGETARCH" in amd64) A=x86_64 ;; arm64) A=aarch_64 ;; *) A=x86_64 ;; esac \
 && curl -fsSL -o /tmp/p.zip "https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-linux-$A.zip" \
 && unzip -o /tmp/p.zip -d /usr/local 'bin/*' 'include/*' \
 && rm /tmp/p.zip

COPY --from=gobuild /go/bin/protoc-gen-go /usr/local/bin/protoc-gen-go

WORKDIR /work
ENTRYPOINT ["buf"]
