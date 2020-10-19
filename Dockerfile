FROM alpine:3

RUN apk add --no-cache curl
COPY kubefwd /

WORKDIR /

ENTRYPOINT ["/kubefwd"]
