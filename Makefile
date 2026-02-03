.PHONY: release build test

release:
	gh workflow run release.yml --field bump=patch

build:
	go build -ldflags="-s -w" -o ralph .

test:
	go test ./...
