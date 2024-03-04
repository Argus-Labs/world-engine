FROM golang:1.21

RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends \
    redis-tools; \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY . .
ENTRYPOINT ["go", "test", "-v", "--tags", "integration", "./e2etestclient"]
