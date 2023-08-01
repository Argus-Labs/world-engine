FROM golang:1.20

WORKDIR /app

COPY . .

ENTRYPOINT ["go", "test", "-v", "--tags", "integration"]
