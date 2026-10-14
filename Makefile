GO ?= $(shell command -v go 2>/dev/null || printf /usr/local/go/bin/go)

.PHONY: build test run ios-project package
build:
	mkdir -p bin
	$(GO) build -trimpath -o bin/neardrop ./cmd/neardrop

test:
	$(GO) test -race ./...
	$(GO) vet ./...

run: build
	./bin/neardrop

ios-project:
	python3 scripts/generate-xcode-project.py

package:
	python3 scripts/package-ios.py
