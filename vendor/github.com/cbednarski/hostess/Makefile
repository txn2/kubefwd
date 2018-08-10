PACKAGES=$(go list ./... | grep -v vendor)
prefix=/usr/local
exec_prefix=$(prefix)
bindir=$(exec_prefix)/bin
datarootdir=$(prefix)/share
datadir=$(datarootdir)
mandir=$(datarootdir)/man

.PHONY: all deps build test gox build-all install clean

all: build test

deps:
	go get github.com/golang/lint/golint
	go get github.com/stretchr/testify/assert
	go get golang.org/x/tools/cmd/cover
	go get

build: deps
	go build cmd/hostess/hostess.go

test:
	go test -coverprofile=coverage.out; go tool cover -html=coverage.out -o coverage.html
	go vet $(PACKAGES)
	golint $(PACKAGES)

gox:
	go get github.com/mitchellh/gox
	gox -build-toolchain

build-all: test
	which gox || make gox
	gox -arch="386 amd64 arm" -os="darwin linux windows" github.com/cbednarski/hostess/cmd/hostess

install: hostess
	mkdir -p $(bindir)
	cp hostess $(bindir)/hostess

clean:
	rm -f ./hostess
	rm -f ./hostess_*
	rm -f ./coverage.*
