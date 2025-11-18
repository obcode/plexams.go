VERSION ?= $(shell date +%Y.%m.%d-%H%M%S)
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -ldflags "\
	-X 'github.com/obcode/plexams.go/cmd.Version=$(VERSION)' \
	-X 'github.com/obcode/plexams.go/cmd.BuildTime=$(BUILD_TIME)' \
	-X 'github.com/obcode/plexams.go/cmd.GitCommit=$(GIT_COMMIT)'"

.PHONY: build
build:
	go build $(LDFLAGS) -o plexams .

.PHONY: install
install:
	go install $(LDFLAGS) .

.PHONY: version
version:
	@echo "Version: $(VERSION)"
	@echo "Git Commit: $(GIT_COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"
