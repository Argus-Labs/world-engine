FROM golang:1.22

WORKDIR /app

COPY . .
ENTRYPOINT ["go", "test", "-v", "--tags", "integration", "./e2etestclient"]
