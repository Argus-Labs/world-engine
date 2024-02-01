FROM golang:1.21

WORKDIR /app

COPY . .
ENTRYPOINT ["go", "test", "-v", "--tags", "integration", "./benchmark-test-client"]
