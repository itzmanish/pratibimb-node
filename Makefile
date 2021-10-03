
GOPATH:=$(shell go env GOPATH)
MODIFY=Mproto/imports/api.proto=github.com/itzmanish/go-micro/v2/api/proto

.PHONY: build
build:

	go build -o pratibimb-go-web *.go

.PHONY: test
test:
	go test -v ./... -cover

.PHONY: docker
docker:
	docker build . -t pratibimb-go-web:latest
