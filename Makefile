.PHONY: release build test test-e2e

release:
	gh workflow run release.yml --field bump=patch

build:
	go build -ldflags="-s -w" -o ralph .

test:
	go test ./...

test-e2e:
	go test -tags e2e -timeout 60m -v -run TestE2E ./...
