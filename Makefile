
GOPATH:=$(shell go env GOPATH)
MODIFY=Mproto/imports/api.proto=github.com/itzmanish/go-micro/v2/api/proto

.PHONY: proto
proto:

	cd proto && buf generate

.PHONY: build
build:

	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -ldflags '-w' -o pratibimb-go

.PHONY: test
test:
	go test -v ./... -cover

.PHONY: docker
docker:
	docker build . -t pratibimb-go:latest
