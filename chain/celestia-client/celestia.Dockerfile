FROM ghcr.io/celestiaorg/celestia-node:0.6.2 AS celestia-node

FROM ghcr.io/celestiaorg/celestia-app:v0.11.0

COPY --from=celestia-node /celestia /

RUN apk update && apk --no-cache add curl jq libc6-compat

COPY entrypoint.sh /

RUN chmod +x /entrypoint.sh 

EXPOSE 26657 26659 9090

ENTRYPOINT ["/entrypoint.sh"]
