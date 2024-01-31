FROM golang:1.21

WORKDIR /usr/src/app

COPY e2e/tester/ e2e/tester/
COPY cardinal cardinal
COPY assert assert

RUN (cd e2e/tester && go mod vendor)
RUN (cd e2e/tester/game && go build -v -o /usr/local/bin/app)

EXPOSE 4040 9020

CMD ["app"]