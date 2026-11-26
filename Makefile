# Build, lint and test entry points. CI (GitHub Actions and the generated
# Jenkinsfile) calls these targets, so local runs match CI exactly.

GO          ?= go
VERSION     ?= $(shell git -c safe.directory="$$PWD" describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X main.version=$(VERSION)
PLATFORMS   ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64
COVERAGE_MIN ?= 80
ACTIONLINT  ?= github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
# The Jenkinsfile in the repository is generated from the CI workflow.
WORKFLOW    := .github/workflows/ci.yml
JENKINSFY   := $(GO) run . -q -label ubuntu-latest=linux $(WORKFLOW)

.PHONY: all lint fmt-check vet golangci tidy-check actionlint changelog-check jenkinsfile \
	jenkinsfile-check test cover dist e2e release release-notes clean

all: lint cover

lint: fmt-check vet golangci tidy-check actionlint changelog-check jenkinsfile-check

fmt-check:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed on:"; echo "$$out"; exit 1; fi

vet:
	$(GO) vet ./...

golangci:
	golangci-lint run ./...

tidy-check:
	$(GO) mod tidy -diff

actionlint:
	$(GO) run $(ACTIONLINT) -color

changelog-check:
	scripts/changelog_test.sh >/dev/null
	scripts/changelog.sh check

jenkinsfile:
	$(JENKINSFY) > Jenkinsfile

jenkinsfile-check:
	@$(JENKINSFY) | diff -u Jenkinsfile - || { echo "Jenkinsfile is stale: run 'make jenkinsfile'"; exit 1; }

test:
	$(GO) test -race -covermode=atomic -coverprofile=coverage.out ./...

cover: test
	$(GO) tool cover -html=coverage.out -o coverage.html
	@total=$$($(GO) tool cover -func=coverage.out | awk '/^total:/ { sub("%", "", $$3); print $$3 }'); \
	echo "Total coverage: $$total% (minimum $(COVERAGE_MIN)%)"; \
	awk -v t="$$total" -v m="$(COVERAGE_MIN)" 'BEGIN { exit !(t >= m) }' || { echo "Coverage below minimum"; exit 1; }

dist:
	@rm -rf dist && mkdir -p dist
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=""; [ "$$os" = windows ] && ext=.exe; \
		out=dist/jenkinsfy_$(VERSION)_$${os}_$${arch}$$ext; \
		echo "building $$out"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -buildvcs=false -ldflags "$(LDFLAGS)" -o $$out . || exit 1; \
	done
	cd dist && sha256sum jenkinsfy_* > SHA256SUMS

e2e:
	test/jenkins/verify.sh all

# make release V=0.2.0: cut the changelog, commit and tag (nothing is pushed).
release:
	@test -n "$(V)" || { echo "usage: make release V=X.Y.Z"; exit 1; }
	scripts/release.sh $(V)

# make release-notes V=0.2.0: print that version's changelog section.
release-notes:
	@scripts/changelog.sh notes $(V)

clean:
	rm -rf dist coverage.out coverage.html
