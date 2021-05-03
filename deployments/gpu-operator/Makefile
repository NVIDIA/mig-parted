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
BUILD_DIR ?= ../..

ifeq ($(IMAGE),)
REGISTRY ?= nvcr.io/nvidia
IMAGE := $(REGISTRY)/k8s-mig-manager
endif

##### Public rules #####

all: ubuntu20.04 ubi8

push:
	$(DOCKER) push "$(IMAGE):$(VERSION)-ubuntu20.04"
	$(DOCKER) push "$(IMAGE):$(VERSION)-ubi8"

push-short:
	$(DOCKER) tag "$(IMAGE):$(VERSION)-ubuntu20.04" "$(IMAGE):$(VERSION)"
	$(DOCKER) push "$(IMAGE):$(VERSION)"

push-latest:
	$(DOCKER) tag "$(IMAGE):$(VERSION)-ubuntu20.04" "$(IMAGE):latest"
	$(DOCKER) push "$(IMAGE):latest"

ubuntu20.04:
	$(DOCKER) build --pull \
		--build-arg VERSION=$(VERSION) \
		--build-arg GOLANG_VERSION=$(GOLANG_VERSION) \
		--build-arg BASE_IMAGE=nvcr.io/nvidia/cuda:11.3.0-base-ubuntu20.04 \
		--tag $(IMAGE):$(VERSION)-ubuntu20.04 \
		--file Dockerfile \
		$(BUILD_DIR)

ubi8:
	$(DOCKER) build --pull \
		--build-arg VERSION=$(VERSION) \
		--build-arg GOLANG_VERSION=$(GOLANG_VERSION) \
		--build-arg BASE_IMAGE=nvcr.io/nvidia/cuda:11.3.0-base-ubi8 \
		--tag $(IMAGE):$(VERSION)-ubi8 \
		--file Dockerfile \
		$(BUILD_DIR)

