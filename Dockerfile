FROM golang:1.10.3

RUN apt-get update
RUN apt-get install -y net-tools
