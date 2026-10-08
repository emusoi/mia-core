.PHONY: install build test site platform

install:
	go install -ldflags='-s -w' ./cmd/mia

build:
	go build -ldflags='-s -w' -o mia ./cmd/mia

test:
	go test ./...

site:
	cd site && bun install --frozen-lockfile && bun build.ts

PLATFORM ?= ../platform

platform: site
	cd site && bun platform.ts $(abspath $(PLATFORM))
