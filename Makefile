DIR := ${CURDIR}

# Image URL to use all building/pushing image targets
IMG ?= signalfx-prometheus-exporter

# Runtime CLI to use for building and pushing images
RUNTIME ?= docker

GO_GCFLAGS ?= -gcflags=all='-N -l'
GO=GO111MODULE=on GOFLAGS=-mod=vendor go
GO_BUILD_RECIPE=CGO_ENABLED=0 $(GO) build $(GO_GCFLAGS)

OUT_DIR ?= bin

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: build

build: signalfx-prometheus-exporter

.PHONY: signalfx-prometheus-exporter
signalfx-prometheus-exporter:
	$(GO_BUILD_RECIPE) -o $(OUT_DIR)/signalfx-prometheus-exporter main.go

run:
	$(GO) run main.go
