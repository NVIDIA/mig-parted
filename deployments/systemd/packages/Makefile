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
VERSION ?= 0.1.0
REVISION ?= 1

DOCKER ?= docker
GOLANG_VERSION ?= 1.15
BUILD_DIR ?= ../../..

ifeq ($(IMAGE),)
REGISTRY ?= nvcr.io/nvidia
IMAGE := $(REGISTRY)/$(NAME)
endif

##### Public rules #####

all: ubuntu18.04 ubuntu20.04 ubi8

ubuntu18.04:
	$(DOCKER) build --pull \
		--build-arg PACKAGE_NAME=$(NAME) \
		--build-arg PACKAGE_VERSION=$(VERSION) \
		--build-arg PACKAGE_REVISION=$(REVISION) \
		--build-arg GOLANG_VERSION=$(GOLANG_VERSION) \
		--build-arg BASE_IMAGE=nvcr.io/nvidia/cuda:11.3.0-base-ubuntu18.04 \
		--tag $(IMAGE):v$(VERSION)-ubuntu18.04 \
		--file Dockerfile.ubuntu \
		$(BUILD_DIR)
	$(DOCKER) run \
		-v $$(pwd)/dist/$(@):/dist \
		$(IMAGE):v$(VERSION)-$(@)

ubuntu20.04:
	$(DOCKER) build --pull \
		--build-arg PACKAGE_NAME=$(NAME) \
		--build-arg PACKAGE_VERSION=$(VERSION) \
		--build-arg PACKAGE_REVISION=$(REVISION) \
		--build-arg GOLANG_VERSION=$(GOLANG_VERSION) \
		--build-arg BASE_IMAGE=nvcr.io/nvidia/cuda:11.3.0-base-ubuntu20.04 \
		--tag $(IMAGE):v$(VERSION)-ubuntu20.04 \
		--file Dockerfile.ubuntu \
		$(BUILD_DIR)
	$(DOCKER) run \
		-v $$(pwd)/dist/$(@):/dist \
		$(IMAGE):v$(VERSION)-$(@)

ubi8:
	$(DOCKER) build --pull \
		--build-arg PACKAGE_NAME=$(NAME) \
		--build-arg PACKAGE_VERSION=$(VERSION) \
		--build-arg PACKAGE_REVISION=$(REVISION) \
		--build-arg GOLANG_VERSION=$(GOLANG_VERSION) \
		--build-arg BASE_IMAGE=nvcr.io/nvidia/cuda:11.3.0-base-ubi8 \
		--tag $(IMAGE):v$(VERSION)-ubi8 \
		--file Dockerfile.ubi8 \
		$(BUILD_DIR)
	$(DOCKER) run \
		-v $$(pwd)/dist/$(@):/dist \
		$(IMAGE):v$(VERSION)-$(@)

