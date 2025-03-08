FROM golang:1.24

WORKDIR /app

COPY . .
ENTRYPOINT ["go", "test", "-v", "--tags", "integration", "./bench"]
