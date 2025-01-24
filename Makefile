version := $(shell cat VERSION)

test:
	golangci-lint run
	go test

build:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/terraform-http-backend

compress:
	gzip -c build/terraform-http-backend > build/terraform-http-backend-$(version)-linux-amd64.gz

.PHONY: test build compress
