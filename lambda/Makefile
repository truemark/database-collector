ARCH           ?= $(shell uname -m)
OS_TYPE        ?= $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH_TYPE      ?= $(subst x86_64,amd64,$(patsubst i%86,386,$(ARCH)))
GOOS           ?= $(shell go env GOOS)
GOARCH         ?= $(shell go env GOARCH)
VERSION        ?= 0.0.1
LDFLAGS        := -X main.Version=$(VERSION)
GOFLAGS        := -ldflags "$(LDFLAGS) -s -w"
BUILD_ARGS      = --build-arg VERSION=$(VERSION)
OUTDIR          = /asset-output

version:
	@echo "Build version $(VERSION) $(GOARCH) $(GOOS) $(ARCH_TYPE)"


.PHONY: go-build
go-build:
	@echo "Build $(OS_TYPE) $(GOOS) $(GOARCH)"
	go build $(GOFLAGS) -o $(OUTDIR)/bootstrap

.PHONY: vendor
vendor: 
	go mod vendor

build:
	go build -mod=vendor .

lambda-build-amd:
	GOOS=$(GOOS) GOARCH=amd64 $(MAKE) go-build -j2

lambda-build-arm:
	GOOS=$(GOOS) GOARCH=arm64 $(MAKE) go-build -j2

lambda-build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(MAKE) go-build -j2

