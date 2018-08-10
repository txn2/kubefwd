buildev:
	docker build -t gonet .

dev: buildev
	docker run -it --rm --privileged -v "$(CURDIR)":/go/src/github.com/txn2/kubefwd -v "$(HOME)/.kube/":/root/.kube/ -w /go/src/github.com/txn2/kubefwd gonet bash

.DEFAULT_GOAL := dev