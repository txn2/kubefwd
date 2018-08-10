FROM alpine:3.7

RUN apk update
RUN apk add curl
COPY kubefwd /

WORKDIR /

ENTRYPOINT ["/kubefwd"]
