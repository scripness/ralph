CURRENT := $(shell grep 'var Version' main.go | sed 's/[^0-9.]//g')
MAJOR := $(word 1,$(subst ., ,$(CURRENT)))
MINOR := $(word 2,$(subst ., ,$(CURRENT)))
PATCH := $(word 3,$(subst ., ,$(CURRENT)))

.PHONY: release build test

release:
	@test -n "$(filter major minor patch,$(BUMP))" || (echo "Usage: make release BUMP=major|minor|patch" && exit 1)
	@if [ "$(BUMP)" = "major" ]; then \
		NEW="$$(($(MAJOR)+1)).0.0"; \
	elif [ "$(BUMP)" = "minor" ]; then \
		NEW="$(MAJOR).$$(($(MINOR)+1)).0"; \
	else \
		NEW="$(MAJOR).$(MINOR).$$(($(PATCH)+1))"; \
	fi; \
	sed -i 's/var Version = "$(CURRENT)"/var Version = "'$$NEW'"/' main.go && \
	git add main.go && \
	git commit -m "release: v$$NEW" && \
	git tag "v$$NEW" && \
	git push && git push --tags && \
	echo "Released v$$NEW"

build:
	go build -ldflags="-s -w" -o ralph .

test:
	go test ./...
