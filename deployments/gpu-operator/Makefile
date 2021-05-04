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

VERSION ?= v0.1.0

DOCKER ?= docker
GOLANG_VERSION ?= 1.15
CUDA_BASE_IMAGE ?= nvcr.io/nvidia/cuda:11.3.0-base
BUILD_DIR ?= ../..

ifeq ($(IMAGE),)
REGISTRY ?= nvcr.io/nvidia
IMAGE := $(REGISTRY)/k8s-mig-manager
endif

##### Public rules #####
DEFAULT_PUSH_TARGET := ubuntu20.04
TARGETS := $(DEFAULT_PUSH_TARGET) ubi8
PUSH_TARGETS := $(patsubst %, push-%, $(TARGETS))
BUILD_TARGETS := $(patsubst %, build-%, $(TARGETS))

.PHONY: $(TARGETS) $(PUSH_TARGETS) $(BUILD_TARGETS) $(TEST_TARGETS)

all: $(TARGETS)

$(PUSH_TARGETS): push-%:
	$(DOCKER) push "$(IMAGE):$(VERSION)-$(*)"

# For the default push target we also push the short and latest tags
push-$(DEFAULT_PUSH_TARGET): push-short push-latest
push-short:
	$(DOCKER) tag "$(IMAGE):$(VERSION)-$(DEFAULT_PUSH_TARGET)" "$(IMAGE):$(VERSION)"
	$(DOCKER) push "$(IMAGE):$(VERSION)"

push-latest:
	$(DOCKER) tag "$(IMAGE):$(VERSION)-$(DEFAULT_PUSH_TARGET)" "$(IMAGE):latest"
	$(DOCKER) push "$(IMAGE):latest"


$(TARGETS): %: build-%
$(BUILD_TARGETS): build-%:
	$(DOCKER) build --pull \
		--build-arg VERSION=$(VERSION) \
		--build-arg GOLANG_VERSION=$(GOLANG_VERSION) \
		--build-arg BASE_IMAGE=$(CUDA_BASE_IMAGE)-$(*) \
		--tag $(IMAGE):$(VERSION)-$(*) \
		--file Dockerfile \
		$(BUILD_DIR)
