ARG GO_IMAGE=golang:1.27.1-bookworm
ARG BASE_IMAGE=gcr.io/distroless/base-debian12

FROM ${GO_IMAGE} AS builder
ARG SHARD_PATH
WORKDIR /src
RUN --mount=type=bind,source=.,target=/src \
    --mount=type=cache,target=/root/.cache/go-build,id=cardinal-gobuild \
    --mount=type=cache,target=/go/pkg/mod,id=cardinal-gomod \
    CGO_ENABLED=0 go build -trimpath -o /out/shard ./${SHARD_PATH}

FROM ${BASE_IMAGE} AS runtime
COPY --from=builder /out/shard /shard
ENTRYPOINT ["/shard"]
