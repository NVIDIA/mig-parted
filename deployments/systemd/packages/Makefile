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
BUILD_DIR ?= ../../..

include $(BUILD_DIR)/versions.mk

ifeq ($(IMAGE),)
REGISTRY ?= nvcr.io/nvidia
IMAGE := $(REGISTRY)/$(NAME)
endif

# strip 'v' from version string
PACKAGE_VERSION := $(VERSION:v%=%)

##### Public rules #####

VALID_TARGETS = tarball ubuntu18.04 ubuntu20.04 ubi8

all: $(VALID_TARGETS)
.PHONY: all $(VALID_TARGETS)

tarball:
	$(DOCKER) build --pull \
		--build-arg PACKAGE_NAME=$(NAME) \
		--build-arg PACKAGE_VERSION=$(PACKAGE_VERSION) \
		--build-arg PACKAGE_REVISION=$(REVISION) \
		--build-arg GOLANG_VERSION=$(GOLANG_VERSION) \
		--tag $(IMAGE):$(vVERSION)-tarball \
		--file Dockerfile.tarball \
		$(BUILD_DIR)
	$(DOCKER) run \
		-v $$(pwd)/dist/$(@):/dist \
		$(IMAGE):$(vVERSION)-$(@)

ubuntu18.04:
	$(DOCKER) build --pull \
		--build-arg PACKAGE_NAME=$(NAME) \
		--build-arg PACKAGE_VERSION=$(PACKAGE_VERSION) \
		--build-arg PACKAGE_REVISION=$(REVISION) \
		--build-arg GOLANG_VERSION=$(GOLANG_VERSION) \
		--build-arg BASE_IMAGE=nvcr.io/nvidia/cuda:${CUDA_VERSION}-base-ubuntu18.04 \
		--tag $(IMAGE):$(vVERSION)-ubuntu18.04 \
		--file Dockerfile.ubuntu \
		$(BUILD_DIR)
	$(DOCKER) run \
		-v $$(pwd)/dist/$(@):/dist \
		$(IMAGE):$(vVERSION)-$(@)

ubuntu20.04:
	$(DOCKER) build --pull \
		--build-arg PACKAGE_NAME=$(NAME) \
		--build-arg PACKAGE_VERSION=$(PACKAGE_VERSION) \
		--build-arg PACKAGE_REVISION=$(REVISION) \
		--build-arg GOLANG_VERSION=$(GOLANG_VERSION) \
		--build-arg BASE_IMAGE=nvcr.io/nvidia/cuda:${CUDA_VERSION}-base-ubuntu20.04 \
		--tag $(IMAGE):$(vVERSION)-ubuntu20.04 \
		--file Dockerfile.ubuntu \
		$(BUILD_DIR)
	$(DOCKER) run \
		-v $$(pwd)/dist/$(@):/dist \
		$(IMAGE):$(vVERSION)-$(@)

ubi8:
	$(DOCKER) build --pull \
		--build-arg PACKAGE_NAME=$(NAME) \
		--build-arg PACKAGE_VERSION=$(PACKAGE_VERSION) \
		--build-arg PACKAGE_REVISION=$(REVISION) \
		--build-arg GOLANG_VERSION=$(GOLANG_VERSION) \
		--build-arg BASE_IMAGE=nvcr.io/nvidia/cuda:${CUDA_VERSION}-base-ubi8 \
		--tag $(IMAGE):$(vVERSION)-ubi8 \
		--file Dockerfile.ubi8 \
		$(BUILD_DIR)
	$(DOCKER) run \
		-v $$(pwd)/dist/$(@):/dist \
		$(IMAGE):$(vVERSION)-$(@)

