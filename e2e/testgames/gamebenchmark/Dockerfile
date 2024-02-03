FROM golang:1.21

ARG CMD_ARG

# Install Perl
RUN apt-get update && apt-get install -y perl && rm -rf /var/lib/apt/lists/*

WORKDIR /usr/src/app

COPY ./ ./
RUN (cd e2e/testgames && go mod vendor)
RUN (cd e2e/testgames/gamebenchmark && go build -v -o /usr/local/bin/app)

EXPOSE 4040 9020

CMD ["app"]