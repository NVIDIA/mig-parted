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

NAME = nvidia-mig-manager
REVISION ?= 1

DOCKER ?= docker
DIST_DIR ?= $(CURDIR)/dist

##### Global variables #####
include $(CURDIR)/versions.mk

ifeq ($(IMAGE),)
REGISTRY ?= nvcr.io/nvidia
IMAGE := $(REGISTRY)/$(NAME)
endif

# strip 'v' from version string
PACKAGE_VERSION := $(VERSION:v%=%)

# replace '-' with '~' in the version string
# e.g. 'x.y.z-rc.1' gets represented as 'x.y.z~rc.1'
PACKAGE_VERSION := $(subst -,~,$(PACKAGE_VERSION))

deb-%: BASE_IMAGE = ubuntu:18.04
deb-%: ARTIFACTS_DIR = $(DIST_DIR)/deb/$(ARCH)
deb-%: DOCKERFILE = $(CURDIR)/deployments/systemd/packages/Dockerfile.deb

rpm-%: BASE_IMAGE = centos:7
rpm-%: ARTIFACTS_DIR = $(DIST_DIR)/rpm/$(ARCH)
rpm-%: DOCKERFILE = $(CURDIR)/deployments/systemd/packages/Dockerfile.rpm

tarball-%: BASE_IMAGE = ubuntu:18.04
tarball-%: ARTIFACTS_DIR = $(DIST_DIR)/tarball/$(ARCH)
tarball-%: DOCKERFILE = $(CURDIR)/deployments/systemd/packages/Dockerfile.tarball

##### Public rules #####

TARGETS = tarball deb rpm

AMD64_TARGETS = $(patsubst %,%-amd64,$(TARGETS))
ARM64_TARGETS = $(patsubst %,%-arm64,$(TARGETS))

$(AMD64_TARGETS): ARCH = amd64
$(ARM64_TARGETS): ARCH = arm64

VALID_TARGETS = $(AMD64_TARGETS) $(ARM64_TARGETS)

all: $(VALID_TARGETS)
.PHONY: all $(VALID_TARGETS) $(TARGETS)

$(VALID_TARGETS): %:
	@echo "Building for $(TARGET_PLATFORM)"
	docker pull --platform=linux/$(ARCH) $(BASE_IMAGE)
	$(DOCKER) build --pull \
		--platform=linux/$(ARCH) \
		--build-arg BASE_IMAGE=$(BASE_IMAGE) \
		--build-arg GOLANG_VERSION=$(GOLANG_VERSION) \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg PACKAGE_NAME=$(NAME) \
		--build-arg PACKAGE_VERSION=$(PACKAGE_VERSION) \
		--build-arg PACKAGE_REVISION=$(REVISION) \
		--tag $(IMAGE):$(VERSION)-$(*) \
		--file $(DOCKERFILE) \
		$(CURDIR)
	$(DOCKER) run \
		--platform=linux/$(ARCH) \
		-v $(ARTIFACTS_DIR):/dist \
		$(IMAGE):$(VERSION)-$(*)
