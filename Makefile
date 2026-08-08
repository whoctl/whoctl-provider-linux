# whoctl-provider-linux
#
# Mutating verbs create and delete real accounts, so every target that runs
# them does it inside a throwaway Alpine container. Nothing here changes the
# workstation.

# The container harness is whoctl's — `make sandbox` there, with this provider
# picked up as a sibling. What is here is this suite and the distros it needs.
#
# scripts/ holds only what is a program in its own right: the assertions, and the
# bridge that hands them to whoctl's harness. Everything else is a recipe, so
# there is one place that knows how to do each thing.
#
# Overridable on the command line: make e2e DISTRO=debian
#
# One distro per package manager: alpine=apk, debian=apt, fedora=dnf, arch=pacman.
# TOOLSET only means something on alpine, the one distro that ships BusyBox's
# account applets instead of shadow-utils.
DISTRO           ?= alpine
TOOLSET          ?= shadow
CONTAINER_ENGINE ?= podman
VERSION          ?= dev

export DISTRO TOOLSET CONTAINER_ENGINE VERSION

.DEFAULT_GOAL := help

## build: build the provider binary
#
# The binary and nothing else. It used to stage the examples here too, and the
# release tars up the whole of bin/ — so every install downloaded nine YAML
# files nobody reads, sitting beside the binary in ~/.whoctl forever. Staging is
# the suite's need, so it is the suite's target.
.PHONY: build
build:
	@mkdir -p bin
	@CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=$(VERSION)" \
		-o bin/whoctl-provider-linux .
	@echo "built bin/whoctl-provider-linux ($(VERSION))"

## examples: stage the manifests the e2e suite applies, for it to mount
.PHONY: examples
examples:
	@rm -rf bin/examples && mkdir -p bin/examples
	@cp examples/*.yaml bin/examples/
	@# An example belongs to the kind it demonstrates, so it lives beside it.
	@# The name carries the path because two kinds are both called apk.
	@find resources -name example.yaml | while read -r e; do \
		cp "$$e" "bin/examples/$$(dirname "$${e#resources/}" | tr / -).yaml"; \
	done

## test: unit tests on the host plus e2e across every distro
.PHONY: test
test:
	@echo "== unit tests (host, read-only)"
	@go test ./...
	@# Every combination runs even after one fails, so a single broken backend
	@# does not hide the state of the other three. Alpine runs twice: it is the
	@# only distro shipping BusyBox's applets instead of shadow-utils.
	@failed=""; \
	for combo in alpine/shadow alpine/busybox debian/shadow fedora/shadow arch/shadow; do \
		distro=$${combo%/*}; toolset=$${combo#*/}; \
		echo; echo "== e2e ($$distro, $$toolset)"; \
		DISTRO=$$distro TOOLSET=$$toolset scripts/e2e-run.sh || failed="$$failed $$combo"; \
	done; \
	echo; \
	if [ -n "$$failed" ]; then echo "FAILED:$$failed"; exit 1; fi; \
	echo "all distros passed"

## unit: unit tests only (safe on the host, reads fixtures and never /etc)
.PHONY: unit
unit:
	@go test ./...

## sandbox: a shell on a throwaway machine, with this provider ready to use
#
# This is the machine the e2e suite runs on, without the suite. It is the only
# place any mutating verb here may be exercised: useradd and apk write to the
# machine they run on, and no --root makes that safe.
.PHONY: sandbox
sandbox:
	@scripts/sandbox.sh $(ARGS)

## e2e: end-to-end tests for one distro (DISTRO=alpine|debian|fedora|arch)
.PHONY: e2e
e2e:
	@scripts/e2e-run.sh

## docs-generate: refresh the generated tables in each kind's page
.PHONY: docs-generate
docs-generate:
	@go run . --docs-generate

## docs: write the documentation bundle a release publishes
.PHONY: docs
docs:
	@go run . --docs-bundle > bundle.json
	@echo "wrote bundle.json"

## fmt: format and vet
.PHONY: fmt
fmt:
	@gofmt -w .
	@go vet ./...

## clean: remove build output
.PHONY: clean
clean:
	@rm -rf bin

## standalone: build and test without the workspace, the way a consumer does
#
# The check lives in whoctl, beside the container harness and for the same
# reason: it is about how a module is consumed, not about what this one manages.
.PHONY: standalone
standalone:
	@../whoctl/scripts/standalone.sh

## help: list the available targets
.PHONY: help
help:
	@echo "whoctl-provider-linux targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //' | awk -F': ' '{printf "  %-9s %s\n", $$1, $$2}'
	@echo
	@echo "Variables: DISTRO=$(DISTRO) TOOLSET=$(TOOLSET) CONTAINER_ENGINE=$(CONTAINER_ENGINE)"
	@echo "DISTRO is alpine|debian|fedora|arch; TOOLSET (shadow|busybox) only applies to alpine."
