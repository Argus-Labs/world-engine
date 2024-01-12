FROM golang:1.21

ARG CMD_ARG

WORKDIR /usr/src/app

COPY internal/e2ebenchmark/tester/game internal/e2ebenchmark/tester/game
COPY cardinal cardinal
COPY assert assert
#COPY internal/e2e/tester/game/vendor internal/e2e/tester/game/vendor
RUN (cd internal/e2ebenchmark/tester/game && go get)
RUN (cd internal/e2ebenchmark/tester/game && go mod vendor)
RUN (cd internal/e2ebenchmark/tester/game && go build -v -o /usr/local/bin/app)

EXPOSE 4040 9020

#CMD app -$CMD_ARG
CMD ["app", "-a"]
#CMD ["app", "-b"]
#CMD ["app", "-d"]
#CMD ["app", "-e"]
#CMD ["app", "-f"]
#CMD ["app", "-g"]