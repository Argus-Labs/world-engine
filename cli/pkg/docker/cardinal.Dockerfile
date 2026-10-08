ARG GO_IMAGE=golang:1.27.1-bookworm
ARG BASE_IMAGE=gcr.io/distroless/base-debian12

# otelc is OpenTelemetry's compile-time instrumentation tool. Its own stage keeps the install in a layer that
# does not depend on the shard source.
FROM ${GO_IMAGE} AS otelc
RUN --mount=type=cache,target=/go/pkg/mod,id=cardinal-gomod \
    GOBIN=/out go install go.opentelemetry.io/otelc/tool/cmd/otelc@v1.1.0

FROM ${GO_IMAGE} AS builder
ARG SHARD_PATH
WORKDIR /src
COPY --from=otelc /out/otelc /usr/local/bin/otelc
# otelc writes its work files (.otelc-build) into the module root, so the source mount is read-write; writes
# are discarded after the step. otelc keeps a private GOCACHE unless GOCACHE is set, so point it at the cache
# mount to keep incremental builds.
RUN --mount=type=bind,source=.,target=/src,rw \
    --mount=type=cache,target=/root/.cache/go-build,id=cardinal-gobuild \
    --mount=type=cache,target=/go/pkg/mod,id=cardinal-gomod \
    CGO_ENABLED=0 GOCACHE=/root/.cache/go-build otelc go build -trimpath -o /out/shard ./${SHARD_PATH}

FROM ${BASE_IMAGE} AS runtime
# otelc injects an SDK setup into main that would claim the global tracer provider before Cardinal's
# telemetry package runs, silently replacing its exporter, sampler, and endpoint defaults. Disabling the
# injected SDK leaves Cardinal as the only exporter; the injected net/http, gRPC, and AWS SDK spans still
# flow through Cardinal's provider. OTEL_LOG_LEVEL quiets otelc's startup logs on stdout.
#
# otelc v1.1.0's gRPC client hook leaves an unsampled span in goroutine-local storage. Every later root span on
# that goroutine, such as cardinal.tick, inherits it and is dropped whenever OTEL_TRACE_SAMPLE_RATE < 1, so the
# gRPC instrumentation stays off.
ENV OTEL_SDK_DISABLED=true \
    OTEL_GO_DISABLED_INSTRUMENTATIONS=grpc \
    OTEL_LOG_LEVEL=warn
COPY --from=builder /out/shard /shard
ENTRYPOINT ["/shard"]
