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

VERSION ?= v0.2.0

DOCKER ?= docker

CUDA_VERSION ?= 11.4.2
GOLANG_VERSION ?= 1.15

ifeq ($(IMAGE_NAME),)
REGISTRY ?= nvcr.io/nvidia/cloud-native
IMAGE_NAME := $(REGISTRY)/k8s-mig-manager
endif

IMAGE_TAG ?= $(VERSION)-$(DIST)
IMAGE = $(IMAGE_NAME):$(IMAGE_TAG)

##### Public rules #####
DEFAULT_PUSH_TARGET := ubuntu20.04
TARGETS := $(DEFAULT_PUSH_TARGET) ubi8
PUSH_TARGETS := $(patsubst %, push-%, $(TARGETS))
BUILD_TARGETS := $(patsubst %, build-%, $(TARGETS))

.PHONY: $(TARGETS) $(PUSH_TARGETS) $(BUILD_TARGETS) $(TEST_TARGETS)

all: $(TARGETS)

$(PUSH_TARGETS): push-%:
	$(DOCKER) push "$(IMAGE_NAME):$(IMAGE_TAG)"

# For the default push target we also push a short tag equal to the version.
# We skip this for the development release
DEVEL_RELEASE_IMAGE_VERSION ?= devel
ifneq ($(strip $(VERSION)),$(DEVEL_RELEASE_IMAGE_VERSION))
push-$(DEFAULT_PUSH_TARGET): push-short
endif
push-short:
	$(DOCKER) tag "$(IMAGE_NAME):$(VERSION)-$(DEFAULT_PUSH_TARGET)" "$(IMAGE_NAME):$(VERSION)"
	$(DOCKER) push "$(IMAGE_NAME):$(VERSION)"

push-%: DIST = $(*)
build-%: DIST = $(*)
build-%: BASE_DIST = $(*)
build-%: DOCKERFILE = $(CURDIR)/deployments/gpu-operator/Dockerfile

# Use a generic build target to build the relevant images
$(TARGETS): %: build-%
$(BUILD_TARGETS): build-%: $(ARTIFACTS_DIR)
	$(DOCKER) build --pull \
		--tag $(IMAGE) \
		--build-arg BASE_DIST="$(BASE_DIST)" \
		--build-arg CUDA_VERSION="$(CUDA_VERSION)" \
		--build-arg GOLANG_VERSION="$(GOLANG_VERSION)" \
		--build-arg VERSION="$(VERSION)" \
		-f $(DOCKERFILE) \
		$(CURDIR)
