.PHONY: build build-docker clean

OUTPUT ?= glm-quota.so

build:
	CGO_ENABLED=1 go build -trimpath -buildmode=c-shared -o $(OUTPUT) .

build-docker:
	docker run --rm -v "$$(pwd):/src" -w /src golang:1.24-bookworm bash -c \
		"CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -buildmode=c-shared -o glm-quota.so ."

clean:
	rm -f $(OUTPUT) glm-quota.h
