FROM alpine:3.8

RUN apk add --no-cache curl
COPY kubefwd /

WORKDIR /

ENTRYPOINT ["/kubefwd"]
