FROM golang:1.21

WORKDIR /usr/src/app

COPY internal/e2e/tester/game internal/e2e/tester/game
COPY cardinal cardinal
COPY assert assert
#COPY internal/e2e/tester/game/vendor internal/e2e/tester/game/vendor
RUN (cd internal/e2e/tester/game && go mod vendor)
RUN (cd internal/e2e/tester/game && go build -v -o /usr/local/bin/app)

EXPOSE 4040 9020

CMD ["app"]