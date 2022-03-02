.PHONY: build push gotest gobuild lint

CONTAINER_ENGINE ?= $(shell which podman >/dev/null 2>&1 && echo podman || echo docker)

IMAGE_NAME := quay.io/goberlec/signalfx-prometheus-exporter
IMAGE_TAG := $(shell git rev-parse --short=7 HEAD)

ifneq (,$(wildcard $(CURDIR)/.docker))
	DOCKER_CONF := $(CURDIR)/.docker
else
	DOCKER_CONF := $(HOME)/.docker
endif

FIRST_GOPATH := $(firstword $(subst :, ,$(shell go env GOPATH)))
GOOPTS       ?=
GOHOSTOS     ?= $(shell go env GOHOSTOS)
GOHOSTARCH   ?= $(shell go env GOHOSTARCH)
GOOS := $(shell go env GOOS)

PKGS          = ./...

GOLANGCI_LINT :=
GOLANGCI_LINT_OPTS ?=
GOLANGCI_LINT_VERSION ?= v1.44.2
# golangci-lint only supports linux, darwin and windows platforms on i386/amd64.
# windows isn't included here because of the path separator being different.
ifeq ($(GOHOSTOS),$(filter $(GOHOSTOS),linux darwin))
	ifeq ($(GOHOSTARCH),$(filter $(GOHOSTARCH),amd64 i386))
		GOLANGCI_LINT := $(FIRST_GOPATH)/bin/golangci-lint
	endif
endif

ifdef GOLANGCI_LINT
$(GOLANGCI_LINT):
	mkdir -p $(FIRST_GOPATH)/bin
	curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/$(GOLANGCI_LINT_VERSION)/install.sh \
		| sed -e '/install -d/d' \
		| sh -s -- -b $(FIRST_GOPATH)/bin $(GOLANGCI_LINT_VERSION)
endif

lint: $(GOLANGCI_LINT)
ifdef GOLANGCI_LINT
	@echo ">> running golangci-lint"
ifdef GO111MODULE
# 'go list' needs to be executed before staticcheck to prepopulate the modules cache.
# Otherwise staticcheck might fail randomly for some reason not yet explained.
#GO111MODULE=$(GO111MODULE) $(GO) list -e -compiled -test=true -export=false -deps=true -find=false -tags= -- ./... > /dev/null
	GO111MODULE=$(GO111MODULE) $(GOLANGCI_LINT) run $(GOLANGCI_LINT_OPTS) $(PKGS)
else
	$(GOLANGCI_LINT) run $(PKGS)
endif
endif

gotest:
	CGO_ENABLED=0 GOOS=$(GOOS) go test $(PKGS)

gobuild: lint gotest
	CGO_ENABLED=0 GOOS=$(GOOS) go build -o signalfx-prometheus-exporter -a -installsuffix cgo main.go

build:
	@DOCKER_BUILDKIT=1 $(CONTAINER_ENGINE) build --no-cache -t $(IMAGE_NAME):latest . --progress=plain
	@$(CONTAINER_ENGINE) tag $(IMAGE_NAME):latest $(IMAGE_NAME):$(IMAGE_TAG)

push:
	@$(CONTAINER_ENGINE) --config=$(DOCKER_CONF) push $(IMAGE_NAME):latest
	@$(CONTAINER_ENGINE) --config=$(DOCKER_CONF) push $(IMAGE_NAME):$(IMAGE_TAG)
