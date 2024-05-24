FROM heroiclabs/nakama-pluginbuilder:3.20.1 AS builder

ENV GO111MODULE on
ENV CGO_ENABLED 1

WORKDIR /nakama

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build --trimpath --buildmode=plugin -o ./plugin.so

FROM heroiclabs/nakama-pluginbuilder:3.20.1-arm AS builder-arm

ENV GO111MODULE on
ENV CGO_ENABLED 1

WORKDIR /nakama

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build --trimpath --buildmode=plugin -o ./plugin.so

FROM heroiclabs/nakama:3.20.1 AS nakama

COPY --from=builder /nakama/plugin.so /nakama/data/modules/
COPY local.yml /nakama/data/

FROM heroiclabs/nakama:3.20.1-arm AS nakama-arm

COPY --from=builder-arm /nakama/plugin.so /nakama/data/modules/
COPY local.yml /nakama/data/