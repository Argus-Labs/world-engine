FROM golang:1.21

WORKDIR /usr/src/app

COPY e2e/testgames/ e2e/testgames/
COPY cardinal cardinal
COPY assert assert

RUN (cd e2e/testgames && go mod vendor)
RUN (cd e2e/testgames/game && go build -v -o /usr/local/bin/app)

EXPOSE 4040 9020

CMD ["app"]