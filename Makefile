# Copyright (c) 2021, NVIDIA CORPORATION.  All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

DOCKER ?= docker

include $(CURDIR)/versions.mk

ifeq ($(IMAGE),)
REGISTRY ?= nvidia
IMAGE_NAME=$(REGISTRY)/mig-parted
endif

EXAMPLES := $(patsubst ./examples/%/,%,$(sort $(dir $(wildcard ./examples/*/))))
EXAMPLE_TARGETS := $(patsubst %,example-%,$(EXAMPLES))

CMDS := $(patsubst ./cmd/%/,%,$(sort $(dir $(wildcard ./cmd/*/))))
CMD_TARGETS := $(patsubst %,cmd-%, $(CMDS))

CHECK_TARGETS := lint
MAKE_TARGETS := binaries build check fmt lint-internal test examples cmds coverage generate vendor check-vendor third-party-notices check-third-party-notices $(CHECK_TARGETS)

TARGETS := $(MAKE_TARGETS) $(EXAMPLE_TARGETS) $(CMD_TARGETS)

DOCKER_TARGETS := $(patsubst %, docker-%, $(TARGETS))
.PHONY: $(TARGETS) $(DOCKER_TARGETS)

GOOS := linux
ifeq ($(VERSION),)
CLI_VERSION = $(LIB_VERSION)$(if $(LIB_TAG),-$(LIB_TAG))
else
CLI_VERSION = $(VERSION)
endif
CLI_VERSION_PACKAGE = $(MODULE)/internal/info

binaries: cmds
ifneq ($(PREFIX),)
cmd-%: COMMAND_BUILD_OPTIONS = -o $(PREFIX)/$(*)
endif
cmds: $(CMD_TARGETS)
$(CMD_TARGETS): cmd-%:
	GOOS=$(GOOS) go build -ldflags "-extldflags=-Wl,-z,lazy -s -w -X $(CLI_VERSION_PACKAGE).gitCommit=$(GIT_COMMIT) -X $(CLI_VERSION_PACKAGE).version=$(CLI_VERSION)" $(COMMAND_BUILD_OPTIONS) $(MODULE)/cmd/$(*)

build:
	GOOS=$(GOOS) go build $(MODULE)/...

examples: $(EXAMPLE_TARGETS)
$(EXAMPLE_TARGETS): example-%:
	GOOS=$(GOOS) go build ./examples/$(*)

all: check test build binary
check: $(CHECK_TARGETS)

# Apply go fmt to the codebase
fmt:
	go list -f '{{.Dir}}' $(MODULE)/... \
		| xargs gofmt -s -l -w

goimports:
	go list -f {{.Dir}} $(MODULE)/... \
		| xargs goimports -local $(MODULE) -w

lint:
	golangci-lint run ./...

vendor:
	go mod tidy
	go mod vendor
	go mod verify

check-vendor: vendor
	git diff --quiet HEAD -- go.mod go.sum vendor

GO_LICENSES = $(CURDIR)/bin/go-licenses

$(GO_LICENSES): deployments/devel/go.mod deployments/devel/go.sum
	cd deployments/devel && GOBIN=$(CURDIR)/bin GOFLAGS=-mod=readonly go install github.com/google/go-licenses/v2

third-party-notices: $(GO_LICENSES)
	@bash hack/generate-third-party-notices.sh

# git diff reports nothing for an untracked path, so the tracked check has to
# run before it or an uncommitted notices file would pass silently.
check-third-party-notices: third-party-notices
	@echo "- Checking if THIRD_PARTY_NOTICES.md is up to date..."
	@git ls-files --error-unmatch THIRD_PARTY_NOTICES.md >/dev/null 2>&1 \
		|| { echo "ERROR: THIRD_PARTY_NOTICES.md is not tracked. Run 'make third-party-notices' and commit the result."; exit 1; }
	@git diff --exit-code HEAD -- THIRD_PARTY_NOTICES.md \
		|| { echo "ERROR: THIRD_PARTY_NOTICES.md is stale. Run 'make third-party-notices' and commit the change."; exit 1; }

COVERAGE_FILE := coverage.out
test: build cmds
	go test -v -coverprofile=$(COVERAGE_FILE) $(MODULE)/cmd/... $(MODULE)/internal/... $(MODULE)/api/... $(MODULE)/pkg/...

coverage: test
	cat $(COVERAGE_FILE) | grep -v "_mock.go" > $(COVERAGE_FILE).no-mocks
	go tool cover -func=$(COVERAGE_FILE).no-mocks

generate:
	go generate $(MODULE)/...

# Generate an image for containerized builds
# Note: This image is local only
.PHONY: .build-image
.build-image:
	make -f deployments/devel/Makefile .build-image

ifeq ($(BUILD_DEVEL_IMAGE),yes)
$(DOCKER_TARGETS): .build-image
.shell: .build-image
endif

$(DOCKER_TARGETS): docker-%:
	@echo "Running 'make $(*)' in container image $(BUILDIMAGE)"
	$(DOCKER) run \
		--rm \
		-e GOCACHE=/tmp/.cache/go \
		-e GOMODCACHE=/tmp/.cache/gomod \
		-e GOLANGCI_LINT_CACHE=/tmp/.cache/golangci-lint \
		-v $(PWD):/work \
		-w /work \
		--user $$(id -u):$$(id -g) \
		$(BUILDIMAGE) \
			make $(*)

# Start an interactive shell using the development image.
PHONY: .shell
.shell:
	$(DOCKER) run \
		--rm \
		-ti \
		-e GOCACHE=/tmp/.cache/go \
		-e GOMODCACHE=/tmp/.cache/gomod \
		-v $(PWD):/work \
		-w /work \
		--user $$(id -u):$$(id -g) \
		$(BUILDIMAGE)

# Deployment targets are forwarded to the Makefile in the following directory
DEPLOYMENT_DIR = deployments/container

DEPLOYMENT_TARGETS = ubuntu20.04 ubi9
BUILD_DEPLOYMENT_TARGETS := $(patsubst %,build-%,$(DEPLOYMENT_TARGETS))
PUSH_DEPLOYMENT_TARGETS := $(patsubst %,push-%,$(DEPLOYMENT_TARGETS))
.PHONY: $(DEPLOYMENT_TARGETS) $(BUILD_DEPLOYMENT_TARGETS) $(PUSH_DEPLOYMENT_TARGETS)

$(BUILD_DEPLOYMENT_TARGETS): build-%:
	@echo "Running 'make $(*)' in $(DEPLOYMENT_DIR)"
	make -f $(DEPLOYMENT_DIR)/Makefile $(*)

$(PUSH_DEPLOYMENT_TARGETS): %:
	@echo "Running 'make $(*)' in $(DEPLOYMENT_DIR)"
	make -f $(DEPLOYMENT_DIR)/Makefile $(*)
