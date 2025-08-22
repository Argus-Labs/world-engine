FROM heroiclabs/nakama-pluginbuilder:3.30.0 AS builder

ENV GO111MODULE on
ENV CGO_ENABLED 1

WORKDIR /nakama/plugin

COPY relay/nakama .
COPY sign /sign

RUN go mod download

RUN go build --trimpath --buildmode=plugin -o ./plugin.so

FROM heroiclabs/nakama-pluginbuilder:3.30.0-arm AS builder-arm

ENV GO111MODULE on
ENV CGO_ENABLED 1

WORKDIR /nakama/plugin

COPY relay/nakama .
COPY sign /sign

RUN go mod download

RUN go build --trimpath --buildmode=plugin -o ./plugin.so

FROM heroiclabs/nakama:3.30.0 AS nakama

RUN apt-get -y update && \
    apt-get -y install --no-install-recommends curl && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /nakama/plugin/plugin.so /nakama/data/modules/
COPY relay/nakama/local.yml /nakama/data/

FROM heroiclabs/nakama:3.30.0-arm AS nakama-arm

COPY --from=builder-arm /nakama/plugin/plugin.so /nakama/data/modules/
COPY relay/nakama/local.yml /nakama/data/
