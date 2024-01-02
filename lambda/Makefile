.PHONY: vendor

vendor:
	go mod vendor

build:
	go build -mod=vendor .

lambda-build:
	go build -tags lambda.norpc -ldflags="-s -w" -o /asset-output/bootstrap main.go
