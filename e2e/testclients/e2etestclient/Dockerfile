FROM golang:1.21

# Update the package index and install Redis tools and Docker
RUN set -eux; \
    apt-get update; \
    # Install Docker CLI
    apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    gnupg \
    lsb-release; \
    # Add Docker’s official GPG key
    curl -fsSL https://download.docker.com/linux/debian/gpg | gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg; \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/debian \
    $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null; \
    apt-get update; \
    apt-get install -y docker-ce-cli; \
    # Clean up unnecessary files
    apt-get clean; \
    rm -rf /var/lib/apt/lists/*; \
    # Install Redis tools
    apt-get update; \
    apt-get install -y --no-install-recommends \
    redis-tools; \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY . .

ENTRYPOINT ["go", "test", "-v", "--tags", "integration", "./e2etestclient"]
