FROM heroiclabs/nakama-pluginbuilder:3.22.0 AS builder

ENV GO111MODULE on
ENV CGO_ENABLED 1

WORKDIR /nakama

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build --trimpath --buildmode=plugin -o ./plugin.so

FROM heroiclabs/nakama:3.22.0 AS nakama

RUN apt-get -y update && apt-get -y install curl

COPY --from=builder /nakama/plugin.so /nakama/data/modules/
COPY local.yml /nakama/data/
 