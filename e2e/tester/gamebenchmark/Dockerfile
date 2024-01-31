FROM golang:1.21

ARG CMD_ARG

WORKDIR /usr/src/app

COPY ./ ./
RUN (cd /e2e/tester && go mod vendor)
RUN (cd /e2e/tester/game/benchmark && go build -v -o /usr/local/bin/app)

EXPOSE 4040 9020

#CMD ["app", "-a"]
#CMD ["app", "-b"]
#CMD ["app", "-c"]
#CMD ["app", "-d"]
#CMD ["app", "-e"]
#CMD ["app", "-f"]
#CMD ["app", "-g"]
CMD ["app", "-h"]