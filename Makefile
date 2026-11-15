IMAGE := tex-dev
GOOS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
GOARCH := $(shell uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')
DOCKER_RUN := docker run --rm \
	-v "$(CURDIR)":/src \
	-v tex-go-build-cache:/root/.cache/go-build \
	-e GOFLAGS=-buildvcs=false \
	$(IMAGE)

.PHONY: build test image

# Cross-compiles ./tex for the host. /usr/local/bin/tex links to it.
build: image
	$(DOCKER_RUN) env CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -trimpath -ldflags='-s -w' -o tex .

# Runs gofmt, go vet, unit tests, and tmux integration tests in Linux.
test: image
	$(DOCKER_RUN) sh -c '\
		unformatted="$$(gofmt -l .)"; \
		if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi; \
		go vet ./... && go test ./...'

image:
	docker build -q -t $(IMAGE) . >/dev/null
