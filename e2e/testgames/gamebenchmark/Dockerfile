FROM golang:1.22

ARG CMD_ARG

WORKDIR /usr/src/app

COPY ./ ./
RUN (cd e2e/testgames && go mod vendor)
RUN (cd e2e/testgames/gamebenchmark && go build -v -o /usr/local/bin/app)

EXPOSE 4040 9020

CMD ["app"]