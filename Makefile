BINARY     := gira
MODULE     := github.com/rafaelescrich/gira
VERSION    := $(shell git -C $(CURDIR) describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS    := -X main.version=$(VERSION)
GOBIN      ?= $(HOME)/go/bin

.PHONY: build test vet fmt install clean

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/gira

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l .

install: build
	mkdir -p $(GOBIN)
	cp $(BINARY) $(GOBIN)/$(BINARY)

clean:
	rm -f $(BINARY)
